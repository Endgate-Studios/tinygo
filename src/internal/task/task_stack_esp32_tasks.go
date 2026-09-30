//go:build scheduler.tasks && (esp32 || esp32s3)

package task

var systemStack uintptr

func (s *state) resume() {
	swapTask(s.sp, &systemStack)
}

func (s *state) pause() {
	newStack := systemStack
	systemStack = 0
	swapTask(newStack, &s.sp)
}

// SystemStack returns the system stack pointer when called from a task stack.
// When called from the system stack, it returns 0.
func SystemStack() uintptr {
	return systemStack
}
