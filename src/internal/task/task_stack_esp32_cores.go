//go:build scheduler.cores && esp32s3

package task

import "runtime/interrupt"

// Returns a pointer to the system stack pointer of the current core.
//
//go:linkname runtime_systemStackPtr runtime.systemStackPtr
func runtime_systemStackPtr() *uintptr

func (s *state) resume() {
	// pause switches back with interrupts disabled, so restore them here.
	mask := interrupt.Disable()
	interrupt.Restore(mask)
	swapTask(s.sp, runtime_systemStackPtr())
	interrupt.Restore(mask)
}

func (s *state) pause() {
	// A GC interrupt must not see the cleared system stack pointer while this
	// core still runs on the goroutine stack.
	mask := interrupt.Disable()
	systemStackPtr := runtime_systemStackPtr()
	newStack := *systemStackPtr
	*systemStackPtr = 0
	swapTask(newStack, &s.sp)
	interrupt.Restore(mask)
}

// SystemStack returns the system stack pointer when called from a task stack.
// When called from the system stack, it returns 0.
func SystemStack() uintptr {
	return *runtime_systemStackPtr()
}

// OnSystemStack returns whether the caller is running on the system stack.
func OnSystemStack() bool {
	return SystemStack() == 0
}
