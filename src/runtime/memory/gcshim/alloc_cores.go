//go:build gc.custom && scheduler.cores

package gcshim

//go:linkname currentCPU runtime.currentCPU
func currentCPU() uint32

// allocsByCore and bytesByCore count allocations by the core that made
// them, under shim.lock, for telling which core's work makes the garbage.
var allocsByCore, bytesByCore [2]uint64

// thisCPU is the core running, 0 or 1.
func thisCPU() uint32 { return currentCPU() & 1 }

func countAlloc(size uintptr) {
	c := thisCPU()
	allocsByCore[c]++
	bytesByCore[c] += uint64(size)
}

// AllocsByCore returns how many allocations each core has made.
func AllocsByCore() (core0, core1 uint64) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return allocsByCore[0], allocsByCore[1]
}

// BytesByCore returns how many bytes each core has allocated.
func BytesByCore() (core0, core1 uint64) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return bytesByCore[0], bytesByCore[1]
}
