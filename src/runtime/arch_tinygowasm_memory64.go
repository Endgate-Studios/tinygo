//go:build tinygo.wasm64

package runtime

// wasmPages counts memory pages: i64 on wasm64 (memory64).
type wasmPages = int64

// wasm_memory_size invokes the "memory.size" instruction, which returns the
// current size to the memory at the given index (always wasmMemoryIndex), in
// pages.
//
//export llvm.wasm.memory.size.i64
func wasm_memory_size(index int32) wasmPages

// wasm_memory_grow invokes the "memory.grow" instruction, which attempts to
// increase the size of the memory at the given index (always wasmMemoryIndex),
// by the delta (in pages). This returns the previous size on success of -1 on
// failure.
//
//export llvm.wasm.memory.grow.i64
func wasm_memory_grow(index int32, delta wasmPages) wasmPages
