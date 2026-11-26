// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// This file has been modified for use by the TinyGo compiler.
"use strict";

(() => {
	const enosys = () => {
		const err = new Error("not implemented");
		err.code = "ENOSYS";
		return err;
	};

	if (!globalThis.fs) {
		let outputBuf = "";
		globalThis.fs = {
			constants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1, O_DIRECTORY: -1 }, // unused
			writeSync(fd, buf) {
				outputBuf += decoder.decode(buf);
				const nl = outputBuf.lastIndexOf("\n");
				if (nl != -1) {
					console.log(outputBuf.substring(0, nl));
					outputBuf = outputBuf.substring(nl + 1);
				}
				return buf.length;
			},
			write(fd, buf, offset, length, position, callback) {
				if (offset !== 0 || length !== buf.length || position !== null) {
					callback(enosys());
					return;
				}
				const n = this.writeSync(fd, buf);
				callback(null, n);
			},
			chmod(path, mode, callback) { callback(enosys()); },
			chown(path, uid, gid, callback) { callback(enosys()); },
			close(fd, callback) { callback(enosys()); },
			fchmod(fd, mode, callback) { callback(enosys()); },
			fchown(fd, uid, gid, callback) { callback(enosys()); },
			fstat(fd, callback) { callback(enosys()); },
			fsync(fd, callback) { callback(null); },
			ftruncate(fd, length, callback) { callback(enosys()); },
			lchown(path, uid, gid, callback) { callback(enosys()); },
			link(path, link, callback) { callback(enosys()); },
			lstat(path, callback) { callback(enosys()); },
			mkdir(path, perm, callback) { callback(enosys()); },
			open(path, flags, mode, callback) { callback(enosys()); },
			read(fd, buffer, offset, length, position, callback) { callback(enosys()); },
			readdir(path, callback) { callback(enosys()); },
			readlink(path, callback) { callback(enosys()); },
			rename(from, to, callback) { callback(enosys()); },
			rmdir(path, callback) { callback(enosys()); },
			stat(path, callback) { callback(enosys()); },
			symlink(path, link, callback) { callback(enosys()); },
			truncate(path, length, callback) { callback(enosys()); },
			unlink(path, callback) { callback(enosys()); },
			utimes(path, atime, mtime, callback) { callback(enosys()); },
		};
	}

	if (!globalThis.process) {
		globalThis.process = {
			getuid() { return -1; },
			getgid() { return -1; },
			geteuid() { return -1; },
			getegid() { return -1; },
			getgroups() { throw enosys(); },
			pid: -1,
			ppid: -1,
			umask() { throw enosys(); },
			cwd() { throw enosys(); },
			chdir() { throw enosys(); },
		}
	}

	if (!globalThis.crypto) {
		throw new Error("globalThis.crypto is not available, polyfill required (crypto.getRandomValues only)");
	}

	if (!globalThis.performance) {
		throw new Error("globalThis.performance is not available, polyfill required (performance.now only)");
	}

	if (!globalThis.TextEncoder) {
		throw new Error("globalThis.TextEncoder is not available, polyfill required");
	}

	if (!globalThis.TextDecoder) {
		throw new Error("globalThis.TextDecoder is not available, polyfill required");
	}

	const encoder = new TextEncoder("utf-8");
	const decoder = new TextDecoder("utf-8");
	let reinterpretBuf = new DataView(new ArrayBuffer(8));
	var logLine = [];
	const wasmExit = {}; // thrown to exit via proc_exit (not an error)

	globalThis.Go = class {
		constructor() {
			this._callbackTimeouts = new Map();
			this._nextCallbackTimeoutID = 1;

			const mem = () => {
				// The buffer may change when requesting more memory.
				return new DataView(this._inst.exports.memory.buffer);
			}

			// Pointer width: 8 on wasm64 (memory64), where pointers and Go's int
			// and uint are i64 (BigInt here), else 4.
			const ptrSize = () => this._ptrSize ??= (this._inst.exports.tinygo_pointer_size ? Number(this._inst.exports.tinygo_pointer_size()) : 4);
			// A pointer or length argument as a number.
			const u = (x) => typeof x === "bigint" ? Number(x) : x >>> 0;
			// A Go int or uint result.
			const goInt = (n) => ptrSize() === 8 ? BigInt(n) : n;
			const loadUint = (addr) => ptrSize() === 8 ? Number(mem().getBigUint64(addr, true)) : mem().getUint32(addr, true);
			const storeUint = (addr, v) => ptrSize() === 8 ? mem().setBigUint64(addr, BigInt(v), true) : mem().setUint32(addr, v, true);

			const unboxValue = (v_ref) => {
				reinterpretBuf.setBigInt64(0, v_ref, true);
				const f = reinterpretBuf.getFloat64(0, true);
				if (f === 0) {
					return undefined;
				}
				if (!isNaN(f)) {
					return f;
				}

				const id = v_ref & 0xffffffffn;
				return this._values[id];
			}


			const loadValue = (addr) => {
				let v_ref = mem().getBigUint64(addr, true);
				return unboxValue(v_ref);
			}

			const boxValue = (v) => {
				const nanHead = 0x7FF80000n;

				if (typeof v === "number") {
					if (isNaN(v)) {
						return nanHead << 32n;
					}
					if (v === 0) {
						return (nanHead << 32n) | 1n;
					}
					reinterpretBuf.setFloat64(0, v, true);
					return reinterpretBuf.getBigInt64(0, true);
				}

				switch (v) {
					case undefined:
						return 0n;
					case null:
						return (nanHead << 32n) | 2n;
					case true:
						return (nanHead << 32n) | 3n;
					case false:
						return (nanHead << 32n) | 4n;
				}

				let id = this._ids.get(v);
				if (id === undefined) {
					id = this._idPool.pop();
					if (id === undefined) {
						id = BigInt(this._values.length);
					}
					this._values[id] = v;
					this._goRefCounts[id] = 0;
					this._ids.set(v, id);
				}
				this._goRefCounts[id]++;
				let typeFlag = 1n;
				switch (typeof v) {
					case "string":
						typeFlag = 2n;
						break;
					case "symbol":
						typeFlag = 3n;
						break;
					case "function":
						typeFlag = 4n;
						break;
				}
				return id | ((nanHead | typeFlag) << 32n);
			}

			const storeValue = (addr, v) => {
				let v_ref = boxValue(v);
				mem().setBigUint64(addr, v_ref, true);
			}

			const loadSlice = (array, len, cap) => {
				return new Uint8Array(this._inst.exports.memory.buffer, array, len);
			}

			const loadSliceOfValues = (array, len, cap) => {
				const a = new Array(len);
				for (let i = 0; i < len; i++) {
					a[i] = loadValue(array + i * 8);
				}
				return a;
			}

			// Browsers fill no view of a shared memory (wasm-cores), and at
			// most 64 KiB at a time: fill a plain buffer, then copy it in.
			const fillRandom = (ptr, len) => {
				const dst = loadSlice(ptr, len);
				for (let off = 0; off < len; off += 65536) {
					const chunk = new Uint8Array(Math.min(65536, len - off));
					crypto.getRandomValues(chunk);
					dst.set(chunk, off);
				}
			};

			const loadString = (ptr, len) => {
				const buf = this._inst.exports.memory.buffer;
				// Browsers decode no view of a shared memory: decode a copy.
				if (typeof SharedArrayBuffer !== "undefined" && buf instanceof SharedArrayBuffer) {
					return decoder.decode(new Uint8Array(buf, ptr, len).slice());
				}
				return decoder.decode(new DataView(buf, ptr, len));
			}

			const timeOrigin = Date.now() - performance.now();
			const wasi_EBADF = 8;
			const wasi_ENOSYS = 52;
			this.importObject = {
				wasi_snapshot_preview1: {
					// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
					fd_write: (fd, iovs_ptr, iovs_len, nwritten_ptr) => {
						iovs_ptr = u(iovs_ptr);
						iovs_len = u(iovs_len);
						nwritten_ptr = u(nwritten_ptr);
						let nwritten = 0;
						if (fd == 1) {
							for (let iovs_i = 0; iovs_i < iovs_len; iovs_i++) {
								let iov_ptr = iovs_ptr + iovs_i * 2 * ptrSize(); // {buf, bufLen}
								let ptr = loadUint(iov_ptr);
								let len = loadUint(iov_ptr + ptrSize());
								nwritten += len;
								for (let i = 0; i < len; i++) {
									let c = mem().getUint8(ptr + i);
									if (c == 13) { // CR
										// ignore
									} else if (c == 10) { // LF
										// write line
										let line = decoder.decode(new Uint8Array(logLine));
										logLine = [];
										console.log(line);
									} else {
										logLine.push(c);
									}
								}
							}
						} else {
							console.error('invalid file descriptor:', fd);
						}
						storeUint(nwritten_ptr, nwritten);
						return goInt(0);
					},
					fd_read: () => wasi_ENOSYS,
					fd_close: () => wasi_ENOSYS,
					fd_fdstat_get: () => wasi_ENOSYS,
					fd_prestat_get: () => wasi_EBADF, // wasi-libc relies on this errno value
					fd_prestat_dir_name: () => wasi_ENOSYS,
					fd_seek: () => wasi_ENOSYS,
					path_open: () => wasi_ENOSYS,
					proc_exit: (code) => {
						this.exited = true;
						this.exitCode = code;
						this._resolveExitPromise();
						throw wasmExit;
					},
					random_get: (bufPtr, bufLen) => {
						bufPtr = u(bufPtr);
						bufLen = u(bufLen);
						fillRandom(bufPtr, bufLen);
						return 0;
					},
				},
				gojs: {
					// func ticks() int64
					"runtime.ticks": () => {
						return BigInt((timeOrigin + performance.now()) * 1e6);
					},

					// func getRandomData(r []byte)
					"runtime.getRandomData": (slice_ptr, slice_len, slice_cap) => {
						slice_ptr = u(slice_ptr);
						slice_len = u(slice_len);
						fillRandom(slice_ptr, slice_len);
					},

					// func sleepTicks(timeout int64)
					"runtime.sleepTicks": (timeout) => {
						// Do not sleep, only reactivate the scheduler after the given
						// timeout, keeping exactly one pending wakeup.
						// Round up to a whole millisecond. Some hosts (Cloudflare
						// Workers) only advance the clock by a timer's delay rounded
						// down to a millisecond, so a fractional remainder would be
						// slept again and again without the clock ever moving.
						const ms = Math.ceil(Number(timeout) / 1e6);
						const due = Date.now() + ms;
						if (this._scheduledWakeup !== undefined) {
							if (this._scheduledWakeupDue <= due) return;
							clearTimeout(this._scheduledWakeup);
						}
						this._scheduledWakeupDue = due;
						this._scheduledWakeup = setTimeout(() => {
							this._scheduledWakeup = undefined;
							if (this.exited) return;
							try {
								this._inst.exports.go_scheduler();
							} catch (e) {
								if (e !== wasmExit) throw e;
							}
						}, ms);
					},

					// func finalizeRef(v ref)
					"syscall/js.finalizeRef": (v_ref) => {
						// Note: TinyGo does not support finalizers so this is only called
						// for one specific case, by js.go:jsString. and can/might leak memory.
						const id = v_ref & 0xffffffffn;
						if (this._goRefCounts?.[id] !== undefined) {
							this._goRefCounts[id]--;
							if (this._goRefCounts[id] === 0) {
								const v = this._values[id];
								this._values[id] = null;
								this._ids.delete(v);
								this._idPool.push(id);
							}
						} else {
							console.error("syscall/js.finalizeRef: unknown id", id);
						}
					},

					// func stringVal(value string) ref
					"syscall/js.stringVal": (value_ptr, value_len) => {
						value_ptr = u(value_ptr);
						value_len = u(value_len);
						const s = loadString(value_ptr, value_len);
						return boxValue(s);
					},

					// func valueGet(v ref, p string) ref
					"syscall/js.valueGet": (v_ref, p_ptr, p_len) => {
						p_ptr = u(p_ptr);
						p_len = u(p_len);
						let prop = loadString(p_ptr, p_len);
						let v = unboxValue(v_ref);
						let result = Reflect.get(v, prop);
						return boxValue(result);
					},

					// func valueSet(v ref, p string, x ref)
					"syscall/js.valueSet": (v_ref, p_ptr, p_len, x_ref) => {
						p_ptr = u(p_ptr);
						p_len = u(p_len);
						const v = unboxValue(v_ref);
						const p = loadString(p_ptr, p_len);
						const x = unboxValue(x_ref);
						Reflect.set(v, p, x);
					},

					// func valueDelete(v ref, p string)
					"syscall/js.valueDelete": (v_ref, p_ptr, p_len) => {
						p_ptr = u(p_ptr);
						p_len = u(p_len);
						const v = unboxValue(v_ref);
						const p = loadString(p_ptr, p_len);
						Reflect.deleteProperty(v, p);
					},

					// func valueIndex(v ref, i int) ref
					"syscall/js.valueIndex": (v_ref, i) => {
						return boxValue(Reflect.get(unboxValue(v_ref), u(i)));
					},

					// valueSetIndex(v ref, i int, x ref)
					"syscall/js.valueSetIndex": (v_ref, i, x_ref) => {
						Reflect.set(unboxValue(v_ref), u(i), unboxValue(x_ref));
					},

					// func valueCall(v ref, m string, args []ref) (ref, bool)
					"syscall/js.valueCall": (ret_addr, v_ref, m_ptr, m_len, args_ptr, args_len, args_cap) => {
						ret_addr = u(ret_addr);
						m_ptr = u(m_ptr);
						m_len = u(m_len);
						args_ptr = u(args_ptr);
						args_len = u(args_len);
						const v = unboxValue(v_ref);
						const name = loadString(m_ptr, m_len);
						const args = loadSliceOfValues(args_ptr, args_len, args_cap);
						try {
							const m = Reflect.get(v, name);
							storeValue(ret_addr, Reflect.apply(m, v, args));
							mem().setUint8(ret_addr + 8, 1);
						} catch (err) {
							storeValue(ret_addr, err);
							mem().setUint8(ret_addr + 8, 0);
						}
					},

					// func valueInvoke(v ref, args []ref) (ref, bool)
					"syscall/js.valueInvoke": (ret_addr, v_ref, args_ptr, args_len, args_cap) => {
						ret_addr = u(ret_addr);
						args_ptr = u(args_ptr);
						args_len = u(args_len);
						try {
							const v = unboxValue(v_ref);
							const args = loadSliceOfValues(args_ptr, args_len, args_cap);
							storeValue(ret_addr, Reflect.apply(v, undefined, args));
							mem().setUint8(ret_addr + 8, 1);
						} catch (err) {
							storeValue(ret_addr, err);
							mem().setUint8(ret_addr + 8, 0);
						}
					},

					// func valueNew(v ref, args []ref) (ref, bool)
					"syscall/js.valueNew": (ret_addr, v_ref, args_ptr, args_len, args_cap) => {
						ret_addr = u(ret_addr);
						args_ptr = u(args_ptr);
						args_len = u(args_len);
						const v = unboxValue(v_ref);
						const args = loadSliceOfValues(args_ptr, args_len, args_cap);
						try {
							storeValue(ret_addr, Reflect.construct(v, args));
							mem().setUint8(ret_addr + 8, 1);
						} catch (err) {
							storeValue(ret_addr, err);
							mem().setUint8(ret_addr + 8, 0);
						}
					},

					// func valueLength(v ref) int
					"syscall/js.valueLength": (v_ref) => {
						return goInt(unboxValue(v_ref).length);
					},

					// valuePrepareString(v ref) (ref, int)
					"syscall/js.valuePrepareString": (ret_addr, v_ref) => {
						ret_addr = u(ret_addr);
						const s = String(unboxValue(v_ref));
						const str = encoder.encode(s);
						storeValue(ret_addr, str);
						storeUint(ret_addr + 8, str.length);
					},

					// valueLoadString(v ref, b []byte)
					"syscall/js.valueLoadString": (v_ref, slice_ptr, slice_len, slice_cap) => {
						slice_ptr = u(slice_ptr);
						slice_len = u(slice_len);
						const str = unboxValue(v_ref);
						loadSlice(slice_ptr, slice_len, slice_cap).set(str);
					},

					// func valueInstanceOf(v ref, t ref) bool
					"syscall/js.valueInstanceOf": (v_ref, t_ref) => {
						return unboxValue(v_ref) instanceof unboxValue(t_ref);
					},

					// func copyBytesToGo(dst []byte, src ref) (int, bool)
					"syscall/js.copyBytesToGo": (ret_addr, dest_addr, dest_len, dest_cap, src_ref) => {
						ret_addr = u(ret_addr);
						dest_addr = u(dest_addr);
						dest_len = u(dest_len);
						let num_bytes_copied_addr = ret_addr;
						let returned_status_addr = ret_addr + ptrSize(); // Address of returned boolean status variable

						const dst = loadSlice(dest_addr, dest_len);
						const src = unboxValue(src_ref);
						if (!(src instanceof Uint8Array || src instanceof Uint8ClampedArray)) {
							mem().setUint8(returned_status_addr, 0); // Return "not ok" status
							return;
						}
						const toCopy = src.subarray(0, dst.length);
						dst.set(toCopy);
						storeUint(num_bytes_copied_addr, toCopy.length);
						mem().setUint8(returned_status_addr, 1); // Return "ok" status
					},

					// copyBytesToJS(dst ref, src []byte) (int, bool)
					// Originally copied from upstream Go project, then modified:
					//   https://github.com/golang/go/blob/3f995c3f3b43033013013e6c7ccc93a9b1411ca9/misc/wasm/wasm_exec.js#L404-L416
					"syscall/js.copyBytesToJS": (ret_addr, dst_ref, src_addr, src_len, src_cap) => {
						ret_addr = u(ret_addr);
						src_addr = u(src_addr);
						src_len = u(src_len);
						let num_bytes_copied_addr = ret_addr;
						let returned_status_addr = ret_addr + ptrSize(); // Address of returned boolean status variable

						const dst = unboxValue(dst_ref);
						const src = loadSlice(src_addr, src_len);
						if (!(dst instanceof Uint8Array || dst instanceof Uint8ClampedArray)) {
							mem().setUint8(returned_status_addr, 0); // Return "not ok" status
							return;
						}
						const toCopy = src.subarray(0, dst.length);
						dst.set(toCopy);
						storeUint(num_bytes_copied_addr, toCopy.length);
						mem().setUint8(returned_status_addr, 1); // Return "ok" status
					},
				}
			};

			// Go 1.20 uses 'env'. Go 1.21 uses 'gojs'.
			// For compatibility, we use both as long as Go 1.20 is supported.
			this.importObject.env = this.importObject.gojs;
		}

		async run(instance) {
			this._inst = instance;
			this._values = [ // JS values that Go currently has references to, indexed by reference id
				NaN,
				0,
				null,
				true,
				false,
				globalThis,
				this,
			];
			this._goRefCounts = []; // number of references that Go has to a JS value, indexed by reference id
			this._ids = new Map();  // mapping from JS values to reference ids
			this._idPool = [];      // unused ids that have been garbage collected
			this.exited = false;    // whether the Go program has exited
			// A wakeup left pending by a previous run would otherwise suppress the
			// first one this run asks for, and the scheduler would never start.
			if (this._scheduledWakeup !== undefined) {
				clearTimeout(this._scheduledWakeup);
				this._scheduledWakeup = undefined;
			}
			this.exitCode = 0;
			// syscall/js.handleEvent reads _pendingEvent and returns early only when
			// it IsNull(). Leaving it `undefined` is not null, so handleEvent falls
			// through to cb.Get("id") and panics with "call of Value.Get on
			// undefined", which surfaces as RuntimeError: unreachable. Upstream Go's
			// wasm_exec.js initializes this in its constructor; this line restores
			// parity. Any resume() that runs without a pending event -- and resume()
			// always spawns a handleEvent goroutine -- depends on it.
			this._pendingEvent = null; // event awaiting dispatch by syscall/js.handleEvent

			if (this._inst.exports._start) {
				let exitPromise = new Promise((resolve, reject) => {
					this._resolveExitPromise = resolve;
				});

				// Run program, but catch the wasmExit exception that's thrown
				// to return back here.
				try {
					this._inst.exports._start();
				} catch (e) {
					if (e !== wasmExit) throw e;
				}

				await exitPromise;
				return this.exitCode;
			} else {
				this._inst.exports._initialize();
			}
		}

		_resume() {
			if (this.exited) {
				throw new Error("Go program has already exited");
			}
			try {
				this._inst.exports.resume();
			} catch (e) {
				if (e !== wasmExit) throw e;
			}
			if (this.exited) {
				this._resolveExitPromise();
			}
		}

		_makeFuncWrapper(id) {
			const go = this;
			return function() {
				const event = { id: id, this: this, args: arguments };
				go._pendingEvent = event;
				go._resume();
				return event.result;
			};
		}
	}
})();

