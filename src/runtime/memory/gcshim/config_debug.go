//go:build gc.custom && !gcshim_stress && memory_debug

package gcshim

const (
	stressEvery = 0
	poison      = true
	strictFree  = true
)
