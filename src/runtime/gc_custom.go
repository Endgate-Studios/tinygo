//go:build gc.custom
// +build gc.custom

package runtime

// This GC strategy allows an external GC to be plugged in instead of the builtin
// implementations.
//
// The interface defined in this file is not stable and can be broken at anytime, even
// across minor versions.
//
// runtime.markStack() must be called at the beginning of any GC cycle. //go:linkname
// on a function without a body can be used to access this internal function.
//
// The custom implementation must provide the following functions in the runtime package
// using the go:linkname directive:
//
// - func initHeap()
// - func alloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer
// - func free(ptr unsafe.Pointer)
// - func markRoots(start, end uintptr)
// - func GC()
// - func SetFinalizer(obj interface{}, finalizer interface{})
// - func ReadMemStats(ms *runtime.MemStats)
// - func markCurrentGoroutineStack(sp uintptr), only with scheduler.cores
//
// The compiler provides the global root ranges used by markRoots.

import (
	"unsafe"
)

const needsStaticHeap = false

// initHeap is called when the heap is first initialized at program start.
func initHeap()

// alloc is called to allocate memory. layout is currently not used.
func alloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer

// free is called to explicitly free a previously allocated pointer.
func free(ptr unsafe.Pointer)

//go:noinline
func freeTaskStack(ptr uintptr) {
	free(unsafe.Pointer(ptr))
}

// markRoots is called with the start and end addresses to scan for references.
func markRoots(start, end uintptr)

// GC is called to explicitly run garbage collection.
func GC()

// SetFinalizer registers a finalizer.
func SetFinalizer(obj interface{}, finalizer interface{})

// ReadMemStats populates m with memory statistics.
func ReadMemStats(ms *MemStats)

// markCurrentGoroutineStack marks the heap object that holds sp, a goroutine
// stack. Only gc_stack_cores.go calls it, so only scheduler.cores needs it.
func markCurrentGoroutineStack(sp uintptr)

func setHeapEnd(newHeapEnd uintptr) {
	// Heap is in custom GC so ignore for when called from wasm initialization.
}

// markRootWord holds the pointer markRoot marks. A global, so marking takes
// no memory during a collection.
var markRootWord uintptr

// markRoot marks the object that root points to, as markRoot of the built-in
// collectors does. scheduler_cores.go marks the tasks that interrupts woke
// with it; the custom collector's hook marks ranges, so give it one word.
func markRoot(addr, root uintptr) {
	markRootWord = root
	start := uintptr(unsafe.Pointer(&markRootWord))
	markRoots(start, start+unsafe.Sizeof(markRootWord))
	markRootWord = 0
}
