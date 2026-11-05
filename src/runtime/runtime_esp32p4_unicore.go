//go:build esp32p4 && !scheduler.cores

package runtime

import "runtime/interrupt"

const numCPU = 1

//go:inline
func currentCPU() uint32 {
	return 0
}

//go:inline
func lockTicks() interrupt.State {
	return 0
}

//go:inline
func unlockTicks(interrupt.State) {
}

func enableInterruptOnCore0(num uint32) {
}

func esp32p4EnterFlashSafeSection() (interrupt.State, bool) {
	return interrupt.Disable(), false
}

func esp32p4ExitFlashSafeSection(state interrupt.State, _ bool) {
	interrupt.Restore(state)
}
