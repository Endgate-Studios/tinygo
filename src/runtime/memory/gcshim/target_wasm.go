//go:build gc.custom && tinygo.wasm

package gcshim

import (
	"unsafe"

	"runtime/memory/collector"
)

const (
	pageSize     = 4096
	worklistLen  = 1024
	wasmPageSize = 64 << 10
	initialHeap  = 256 << 10 // committed at start-up, beyond metadata
)

// The heap starts at __heap_base and grows with memory.grow. Metadata for maxHeapSize (about 1/60)
// is set at start, because wasm memory grows only at the end (WebAssembly spec, memory.grow).
//
//go:extern __heap_base
var heapBaseSymbol [0]byte

//export llvm.wasm.memory.size.i32
func wasmMemorySize(index int32) int32

//export llvm.wasm.memory.grow.i32
func wasmMemoryGrow(index int32, delta int32) int32

// markStack scans the system stack (src/runtime/gc_stack_portable.go). Goroutine stacks are heap
// objects, and the GC scans them through the task structures that refer to them.
//
//go:linkname markStack runtime.markStack
func markStack()

func checkAllocContext() {}

// wasm has one thread, so there is nothing to stop.
func markAllRoots() {
	markStack()
	markGlobals()
}

func resumeWorld() {}

func memoryEnd() uintptr {
	return uintptr(wasmMemorySize(0)) * wasmPageSize
}

// commitTo grows linear memory so that addr is accessible.
func commitTo(addr uintptr) bool {
	end := memoryEnd()
	if addr <= end {
		return true
	}
	pages := (addr - end + wasmPageSize - 1) / wasmPageSize
	return wasmMemoryGrow(0, int32(pages)) >= 0
}

func initArena() error {
	start := uintptr(unsafe.Pointer(&heapBaseSymbol))
	limit := start + maxHeapSize
	cfg := collectorConfig()
	if !commitTo(collector.MetadataEnd(start, limit, cfg) + initialHeap) {
		return collector.ErrHeapTooSmall
	}
	return shim.heap.Init(start, memoryEnd(), limit, cfg)
}

// growArena grows the heap by at least min bytes and doubles it if possible.
func growArena(min uintptr) bool {
	start, end := shim.heap.Bounds()
	step := end - start
	if step < min+pageSize {
		step = min + pageSize
	}
	target := end + step
	if limit := shim.heap.Limit(); target > limit {
		target = limit
	}
	if target <= end || !commitTo(target) {
		return false
	}
	return shim.heap.Grow(memoryEnd())
}
