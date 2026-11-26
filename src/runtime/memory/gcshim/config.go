//go:build gc.custom && !gcshim_stress && !memory_debug

package gcshim

const (
	stressEvery = 0     // collect on each Nth allocation, 0 disables it
	poison      = false // fill freed memory with 0xDEADBEEF
	strictFree  = false // panic on invalid or double free inside the heap
)
