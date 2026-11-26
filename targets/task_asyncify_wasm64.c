// Calls fn(args) for task_asyncify_wasm.S on wasm64 (see wasm_ptr.h).
void tinygo_call_entry(void *args, void (*fn)(void *)) {
	fn(args);
}
