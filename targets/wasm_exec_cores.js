// TinyGo wasm-cores: core 0 on this thread, every other core a Worker on
// the same module and the same shared memory (a SharedArrayBuffer: the page
// must be cross-origin isolated). Needs wasm_exec.js's Go first.
"use strict";

(() => {
	const isNode = typeof process !== "undefined" && process.versions && process.versions.node;

	// The memory import's limits, from the import section.
	// WebAssembly core spec 5.5.10 (imports) and 5.3.7 (limits).
	function memoryLimits(bytes) {
		bytes = new Uint8Array(bytes);
		let i = 8;
		// LEB128 as a number (a memory64 limit can pass 32 bits).
		const u32 = () => { let r = 0, m = 1, b; do { b = bytes[i++]; r += (b & 0x7f) * m; m *= 128; } while (b & 0x80); return r; };
		const skipName = () => { const n = u32(); i += n; };
		while (i < bytes.length) {
			const id = bytes[i++], size = u32(), end = i + size;
			if (id === 2) {
				for (let n = u32(); n > 0; n--) {
					skipName(); skipName();
					const kind = bytes[i++];
					if (kind === 0) u32();
					else if (kind === 1) { i++; const f = bytes[i++]; u32(); if (f & 1) u32(); }
					else if (kind === 2) {
						// Flags: 1 a maximum, 2 shared, 4 64-bit addresses (memory64).
						const f = bytes[i++]; const initial = u32(); const maximum = (f & 1) ? u32() : initial;
						return (f & 4) ? { initial, maximum, address: "i64" } : { initial, maximum };
					}
					else if (kind === 3) i += 2;
				}
			}
			i = end;
		}
		throw new Error("wasm-cores: the module imports no memory");
	}

	// coreCount is how many cores to run: the machine's threads.
	function coreCount() {
		if (isNode) return Number(process.env.WASMCORES_CORES) || require("os").availableParallelism();
		return Number(globalThis.WASMCORES_CORES) || (globalThis.navigator && navigator.hardwareConcurrency) || 4;
	}

	// endProgram sets the exit word (the code plus one) and wakes its waiter.
	function endProgram(memory, addr, code) {
		const words = new Int32Array(memory.buffer);
		Atomics.store(words, addr >>> 2, code + 1);
		Atomics.notify(words, addr >>> 2);
	}

	// coreImports are a core's imports: wasm_exec.js's, on the shared memory.
	// A core that exits (os.Exit, a fatal panic) ends the whole program.
	function coreImports(go, memory, startCore) {
		const imports = go.importObject;
		imports.env.memory = memory;
		imports.tinygo_cores = { start_core: startCore, core_count: coreCount };
		const procExit = imports.wasi_snapshot_preview1.proc_exit;
		imports.wasi_snapshot_preview1.proc_exit = (code) => {
			if (go._exitWord !== undefined) endProgram(memory, go._exitWord, code);
			return procExit(code);
		};
		return imports;
	}

	// enterCore runs a secondary core: its own instance, stack and
	// thread-local block.
	function enterCore(go, instance, core, stackTop, tls) {
		go._inst = instance;
		// wasm_exec.js's own state for syscall/js, as its run sets it up. JS
		// values are this thread's: a js.Value isn't to cross cores.
		go._values = [NaN, 0, null, true, false, globalThis, go];
		go._goRefCounts = [];
		go._ids = new Map();
		go._idPool = [];
		go.exited = false;
		go.exitCode = 0;
		go._pendingEvent = null;
		instance.exports.__stack_pointer.value = stackTop;
		instance.exports.__wasm_init_tls(tls);
		instance.exports.tinygo_core_entry(core);
	}

	class GoCores extends globalThis.Go {
		constructor(workerScript) {
			super();
			this.workerScript = workerScript;
			this.workers = [];
		}

		// start instantiates bytes as core 0 and runs it to its end.
		async start(bytes) {
			const module = await WebAssembly.compile(bytes);
			const limits = memoryLimits(bytes);
			try {
				this.memory = new WebAssembly.Memory({ ...limits, shared: true });
			} catch (e) {
				if (!limits.address) throw e;
				this.memory = new WebAssembly.Memory({ ...limits, initial: BigInt(limits.initial), maximum: BigInt(limits.maximum), shared: true });
			}
			const imports = coreImports(this, this.memory, (core, stackTop, tls) => this.startCore(module, core, stackTop, tls));
			const instance = await WebAssembly.instantiate(module, imports);
			if (!instance.exports.tinygo_cores_exit_word) {
				// Built without the cores scheduler: main ends on core 0.
				try {
					return await this.run(instance);
				} finally {
					for (const w of this.workers) w.terminate();
				}
			}
			this._exitWord = Number(instance.exports.tinygo_cores_exit_word());
			try {
				// _start only starts the cores; main ends on one of them.
				const done = this.run(instance);
				this.waitExit().then((code) => {
					if (this.exited) return;
					this.exited = true;
					this.exitCode = code;
					this._resolveExitPromise();
				});
				return await done;
			} finally {
				for (const w of this.workers) w.terminate();
			}
		}

		// waitExit waits, without blocking this thread, for the exit word.
		async waitExit() {
			const words = new Int32Array(this.memory.buffer);
			const i = this._exitWord >>> 2;
			for (;;) {
				const v = Atomics.load(words, i);
				if (v !== 0) return v - 1;
				const r = Atomics.waitAsync(words, i, 0);
				if (r.async) await r.value;
			}
		}

		startCore(module, core, stackTop, tls) {
			const data = { module, memory: this.memory, core, stackTop, tls };
			let w;
			if (isNode) {
				const { Worker } = require("worker_threads");
				w = new Worker(this.workerScript, { workerData: data });
			} else {
				w = new Worker(this.workerScript);
				w.postMessage(data);
			}
			// A core that dies (a trap) ends the program: the others may wait on it.
			const died = (e) => {
				console.error("wasm-cores: core", core, e);
				if (this._exitWord !== undefined) endProgram(this.memory, this._exitWord, 2);
			};
			if (w.on) {
				w.on("error", died);
				w.on("exit", (code) => { if (code !== 0) died("exited with " + code); });
			} else {
				w.onerror = died;
			}
			this.workers.push(w);
		}
	}

	// runCore is a Worker's side: the core it was started as.
	async function runCore(data) {
		const go = new globalThis.Go();
		const imports = coreImports(go, data.memory, () => { throw new Error("wasm-cores: only core 0 starts cores"); });
		const instance = await WebAssembly.instantiate(data.module, imports);
		if (instance.exports.tinygo_cores_exit_word) go._exitWord = Number(instance.exports.tinygo_cores_exit_word());
		try {
			enterCore(go, instance, data.core, data.stackTop, data.tls);
		} catch (e) {
			if (!go.exited) throw e;
		}
	}

	globalThis.GoCores = GoCores;
	globalThis.goCoresRunCore = runCore;
})();
