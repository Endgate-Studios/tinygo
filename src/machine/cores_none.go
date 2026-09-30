//go:build !scheduler.cores

package machine

// LockCore pins the calling goroutine to the given core. Without the cores
// scheduler, only core 0 exists.
func LockCore(core int) {
	if core != 0 {
		panic("machine: core out of range")
	}
}

// UnlockCore lets the calling goroutine run on any core again.
func UnlockCore() {
}
