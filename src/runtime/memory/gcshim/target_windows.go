//go:build gc.custom && windows && amd64 && scheduler.tasks

package gcshim

import "runtime/memory/collector"

const (
	pageSize    = 4096
	worklistLen = 1024
	maxHeapSize = 256 << 20
	initialHeap = 1 << 20 // beyond metadata
)

// preinit reserves and commits 1 GB at heapStart, and main sets stackTop
// (src/runtime/runtime_windows.go). Windows backs a page on first touch.
//
//go:extern runtime.heapStart
var runtimeHeapStart uintptr

//go:extern runtime.stackTop
var runtimeStackTop uintptr

//go:linkname taskOnSystemStack internal/task.OnSystemStack
func taskOnSystemStack() bool

//go:linkname taskSystemStack internal/task.SystemStack
func taskSystemStack() uintptr

// scanCurrentStack pushes the callee-saved registers and calls tinygo_scanstack(sp)
// (src/runtime/asm_amd64_windows.S).
//
//export tinygo_scanCurrentStack
func scanCurrentStack()

// scanstack is defined by src/runtime/gc_stack_raw.go only for the built-in collectors.
//
//export tinygo_scanstack
func scanstack(sp uintptr) {
	if taskOnSystemStack() {
		markRoots(sp, runtimeStackTop)
		return
	}
	shim.heap.MarkStack(sp)
}

func checkAllocContext() {}

// The tasks scheduler is cooperative, so no other goroutine runs during a collection.
func markAllRoots() {
	scanCurrentStack()
	if !taskOnSystemStack() {
		markRoots(taskSystemStack(), runtimeStackTop)
	}
	markGlobals()
}

func resumeWorld() {}

func initArena() error {
	start := runtimeHeapStart
	limit := start + maxHeapSize
	cfg := collectorConfig()
	return shim.heap.Init(start, collector.MetadataEnd(start, limit, cfg)+initialHeap, limit, cfg)
}

// growArena doubles the heap, up to maxHeapSize. The memory is already committed.
func growArena(need uintptr) bool {
	start, end := shim.heap.Bounds()
	target := end + max(end-start, need+pageSize)
	target = min(target, shim.heap.Limit())
	if target <= end {
		return false
	}
	return shim.heap.Grow(target)
}
