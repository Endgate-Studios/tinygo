//go:build scheduler.cores && tinygo.wasm

package task

// Pause suspends the current task and returns to the scheduler.
func Pause() {
	lockScheduler()
	PauseLocked()
}

// PauseLocked is Pause with the scheduler lock held. The task unwinds to
// the scheduler with the lock still held, as the cores scheduler expects.
func PauseLocked() {
	current := currentTask
	if *current.state.canaryPtr != stackCanary {
		runtimeFatal("goroutine stack overflow")
	}
	if current.RunState == RunStateResuming {
		// Another core already made it ready again.
		current.RunState = RunStateRunning
		unlockScheduler()
		return
	}
	current.RunState = RunStatePaused
	current.state.unwind()
}

//go:linkname lockScheduler runtime.lockScheduler
func lockScheduler()

//go:linkname unlockScheduler runtime.unlockScheduler
func unlockScheduler()

// pinToCore keeps a goroutine on the core it first ran on. LLVM keeps a
// thread-local's address across calls, and a goroutine paused by asyncify
// would resume with the old core's addresses if it moved.
func pinToCore(t *Task) {
	if t.Affinity.Core == 0 {
		t.Affinity.Core = uint8(currentCPU() + 1)
	}
}

//go:linkname currentCPU runtime.currentCPU
func currentCPU() uint32

// cannotPause is whether there's no goroutine to pause: the scheduler,
// on a core's own stack, taking a lock another core holds.
func cannotPause() bool { return currentTask == nil }

//go:linkname spinLoopWait runtime.spinLoopWait
func spinLoopWait()

//go:linkname runtime_systemStackPtr runtime.systemStackPtr
func runtime_systemStackPtr() *uintptr

// The system stack pointer is set only while the core runs a goroutine.
func enterFromSystemStack() { *runtime_systemStackPtr() = getCurrentStackPointer() }

func backOnSystemStack() { *runtime_systemStackPtr() = 0 }

// SystemStack returns the system stack pointer when called from a task stack.
// When called from the system stack, it returns 0.
func SystemStack() uintptr {
	return *runtime_systemStackPtr()
}

// OnSystemStack returns whether the caller is running on the system stack.
func OnSystemStack() bool {
	return SystemStack() == 0
}

// QueuedPerCore counts q's tasks by the core they're pinned to (index 0:
// not pinned), for the scheduler's debug dump.
func (q *Queue) QueuedPerCore() (counts [9]int) {
	for t := q.head; t != nil; t = t.Next {
		core, assigned := t.AssignedCore()
		if !assigned {
			counts[0]++
		} else if core < 8 {
			counts[core+1]++
		}
	}
	return counts
}

// pendingStack is an exited goroutine's stack, freed at the core's next
// Resume: freeing takes the heap's lock, which may not be waited on while
// the scheduler's is held (a core holding the heap's may want it).
//
//go:threadlocal
var pendingStack uintptr

func freeStackLater(stack uintptr) {
	freePendingStack()
	pendingStack = stack
}

// freePendingStack frees it, the scheduler's lock not held.
func freePendingStack() {
	if pendingStack != 0 {
		stack := pendingStack
		pendingStack = 0
		runtime_freeTaskStack(stack)
	}
}
