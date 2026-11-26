//go:build !scheduler.cores

package runtime

func lockOSThread() {
}

func unlockOSThread() {
}

// procPin saves one interrupt state per core.
const procPinSlots = 1

func procPinSlot() uint32 {
	return 0
}
