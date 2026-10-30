//go:build gc.custom && !scheduler.cores

package gcshim

func countAlloc(size uintptr) {}

// thisCPU is the core running: there is one.
func thisCPU() uint32 { return 0 }

// BytesByCore returns all bytes allocated as core 0's: there is one core.
func BytesByCore() (core0, core1 uint64) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return shim.heap.Stats().TotalAlloc, 0
}

// AllocsByCore returns all allocations as core 0's: there is one core.
func AllocsByCore() (core0, core1 uint64) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return shim.heap.Stats().Mallocs, 0
}
