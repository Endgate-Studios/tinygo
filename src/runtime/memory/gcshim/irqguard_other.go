//go:build gc.custom && !(baremetal && esp32p4)

package gcshim

// checkInterrupts has nothing to check: see irqguard_esp32p4.go.
func checkInterrupts() {}
