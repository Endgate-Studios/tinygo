//go:build scheduler.cores

package machine

// LockCore pins the calling goroutine to the given core and moves it there.
// A pinned goroutine waits for its core, also when another core is idle.
func LockCore(core int) {
	if !lockCore(core) {
		panic("machine: core out of range")
	}
}

// UnlockCore lets the calling goroutine run on any core again.
func UnlockCore() {
	unlockCore()
}

//go:linkname lockCore runtime.machineLockCore
func lockCore(core int) bool

//go:linkname unlockCore runtime.machineUnlockCore
func unlockCore()
