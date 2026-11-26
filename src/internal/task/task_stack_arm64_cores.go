//go:build scheduler.cores && arm64

package task

// The system stack pointer is per core, in the runtime.
//
//go:linkname runtime_systemStackPtr runtime.systemStackPtr
func runtime_systemStackPtr() *uintptr

func (s *state) resume() {
	swapTaskCores(s.sp, runtime_systemStackPtr(), nil)
}

// pause clears the core's system stack pointer only once it's on that stack, so a GC pause
// in between still scans it. tinygo_swapTaskRISC does the same.
func (s *state) pause() {
	systemStackPtr := runtime_systemStackPtr()
	swapTaskCores(*systemStackPtr, &s.sp, systemStackPtr)
}

//export tinygo_swapTaskCores
func swapTaskCores(newStack uintptr, oldStack, clearStack *uintptr)

// SystemStack returns the system stack pointer when called from a task stack.
// When called from the system stack, it returns 0.
func SystemStack() uintptr {
	return *runtime_systemStackPtr()
}
