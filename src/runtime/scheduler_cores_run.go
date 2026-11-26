//go:build scheduler.cores && !tinygo.wasm

package runtime

// This function is called on the first core in the system. It will wake up the
// other cores when ready.
func run() {
	initRand()
	initHeap()

	go func() {
		// Package initializers are currently run single-threaded.
		// This might help with registering interrupts and such.
		initAll()

		// After package initializers have finished, start all the other cores.
		startSecondaryCores()
		secondaryCoresReady.Store(1)

		// Run main.main.
		callMain()

		// main.main has exited, so the program should exit.
		mainExited.Store(1)
	}()

	// The scheduler must always be entered while the scheduler lock is taken.
	schedulerLock.Lock()
	scheduler(false)
	schedulerLock.Unlock()
}

// NumCPU returns the number of CPU cores on this system.
func NumCPU() int {
	return numCPU
}
