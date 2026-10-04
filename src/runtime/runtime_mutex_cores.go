//go:build scheduler.cores

package runtime

import "internal/task"

type runtimeSpinLock struct {
	state task.Uint32
}

func (lock *runtimeSpinLock) Lock() {
	for !lock.state.CompareAndSwap(0, 1) {
	}
}

func (lock *runtimeSpinLock) Unlock() {
	lock.state.Store(0)
}

type timerLock = runtimeSpinLock

// chanLock is a spin lock: an interrupt and the scheduler take channel locks
// to wake a task, and neither can pause. Holders keep interrupts disabled and
// never pause, so a holder on the other core lets go soon.
type chanLock = runtimeSpinLock
type synctestLock = runtimeSpinLock
