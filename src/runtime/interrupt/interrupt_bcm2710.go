//go:build bcm2710

package interrupt

import "device/arm64"

// State represents the previous global interrupt state.
type State uintptr

// Disable masks IRQ and FIQ and returns the previous DAIF.
func Disable() (state State) {
	state = State(arm64.AsmFull("mrs {}, daif", nil))
	arm64.Asm("msr daifset, #3")
	return state
}

// Restore puts DAIF back to what Disable returned.
func Restore(state State) {
	arm64.AsmFull("msr daif, {state}", map[string]interface{}{"state": uintptr(state)})
}

// In returns whether the CPU is handling an interrupt. No interrupts are taken yet.
func In() bool {
	return false
}
