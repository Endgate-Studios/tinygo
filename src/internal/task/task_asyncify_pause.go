//go:build scheduler.asyncify

package task

// Pause suspends the current task and returns to the scheduler.
// This function may only be called when running on a goroutine stack, not when running on the system stack.
func Pause() {
	if *currentTask.state.canaryPtr != stackCanary {
		runtimeFatal("stack overflow")
	}

	currentTask.state.unwind()
}

func pinToCore(t *Task) {}

func freeStackLater(stack uintptr) { runtime_freeTaskStack(stack) }

func freePendingStack() {}

// enterFromSystemStack saves the system stack pointer for the GC.
func enterFromSystemStack() { saveStackPointer() }

func backOnSystemStack() {}

//go:linkname saveStackPointer runtime.saveStackPointer
func saveStackPointer()

// OnSystemStack returns whether the caller is running on the system stack.
func OnSystemStack() bool {
	// If there is not an active goroutine, then this must be running on the system stack.
	return Current() == nil
}
