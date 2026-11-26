// A core of the cores glue; its console lines go to the server too.
function post(path, body) {
	const x = new XMLHttpRequest();
	x.open("POST", path, false);
	x.send(body);
}
for (const k of ["log", "error", "warn"]) {
	console[k] = (...a) => post("/log", a.join(" "));
}
importScripts("/wasm_exec.js", "/wasm_exec_cores.js");
onmessage = (e) => {
	onmessage = null;
	goCoresRunCore(e.data).catch((err) => console.error("worker:", err && err.stack || err));
};
