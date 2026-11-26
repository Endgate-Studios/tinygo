//go:build tinygo.wasm && !tinygo.wasm64

package runtime

// wasmPages counts memory pages: i32 on wasm32.
type wasmPages = int32

// wasm_memory_size invokes the "memory.size" instruction, which returns the
// current size to the memory at the given index (always wasmMemoryIndex), in
// pages.
//
//export llvm.wasm.memory.size.i32
func wasm_memory_size(index int32) wasmPages

// wasm_memory_grow invokes the "memory.grow" instruction, which attempts to
// increase the size of the memory at the given index (always wasmMemoryIndex),
// by the delta (in pages). This returns the previous size on success of -1 on
// failure.
//
//export llvm.wasm.memory.grow.i32
func wasm_memory_grow(index int32, delta wasmPages) wasmPages
