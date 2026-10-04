//go:build gc.custom && baremetal && (esp32 || esp32s3 || esp32c3) && !scheduler.cores

package gcshim

import "unsafe"

//go:extern _stack_top
var stackTopSymbol [0]byte

//go:linkname taskOnSystemStack internal/task.OnSystemStack
func taskOnSystemStack() bool

//go:linkname taskSystemStack internal/task.SystemStack
func taskSystemStack() uintptr

// scanCurrentStack is the target assembly routine (src/device/esp/esp32s3.S). It puts the registers,
// or all Xtensa register windows, on the stack and calls tinygo_scanstack(sp).
//
//export tinygo_scanCurrentStack
func scanCurrentStack()

// scanstack gets the stack pointer after the register spill. TinyGo 0.42 defines it only for
// the built-in collectors (src/runtime/gc_stack_raw.go), so gcshim supplies it.
//
//export tinygo_scanstack
func scanstack(sp uintptr) {
	if taskOnSystemStack() {
		markRoots(sp, stackTop())
		return
	}
	// A goroutine stack is a heap object. The mark keeps it and scans only its live part above sp.
	shim.heap.MarkStack(sp)
}

// markAllRoots does the same as gcMarkReachable in src/runtime/gc_stack_raw.go. It scans
// the current stack, the system stack and then the globals.
func markAllRoots() {
	scanCurrentStack()
	if !taskOnSystemStack() {
		markRoots(taskSystemStack(), stackTop())
	}
	markGlobals()
}

// resumeWorld does nothing on a single core.
func resumeWorld() {}

func stackTop() uintptr {
	return uintptr(unsafe.Pointer(&stackTopSymbol))
}
