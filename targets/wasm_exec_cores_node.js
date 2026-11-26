// Runs a TinyGo wasm-cores module under Node: core 0 here, the other cores
// in worker_threads running this same file.
"use strict";

const { isMainThread, workerData } = require("worker_threads");

globalThis.require = require;
globalThis.fs = require("fs");
globalThis.performance = {
	now() {
		const [sec, nsec] = process.hrtime();
		return sec * 1000 + nsec / 1000000;
	},
};

require("./wasm_exec");
require("./wasm_exec_cores");

if (isMainThread) {
	if (process.argv.length < 3) {
		console.error("usage: wasm_exec_cores_node.js [wasm binary] [arguments]");
		process.exit(1);
	}
	const go = new GoCores(__filename);
	go.start(fs.readFileSync(process.argv[2])).then((code) => process.exit(code)).catch((err) => {
		console.error(err);
		process.exit(1);
	});
} else {
	// A worker's console is passed to the main thread later, and lost if the
	// program ends first: its lines are written at once.
	console.log = (...args) => fs.writeSync(1, args.join(" ") + "\n");
	goCoresRunCore(workerData).catch((err) => {
		console.error(err);
		process.exit(1);
	});
}
