//go:build esp32p4 && usbotg

package machine

// usbLeaveBus drops off USB, and waits for the host to see it, so it finds the ROM's port as new.
// It can't use timers, as interrupts are off: a register read takes at least 100 ns.
func usbLeaveBus() {
	otg.DCTL.SetBits(dctlSDIS)
	for i := 0; i < 5_000_000; i++ {
		_ = otg.DCTL.Get()
	}
}
