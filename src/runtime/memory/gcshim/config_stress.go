//go:build gc.custom && gcshim_stress

package gcshim

const (
	stressEvery = 1
	poison      = true
	strictFree  = true
)
