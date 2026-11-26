//go:build xtensa

package runtime

import "device"

const GOARCH = "arm" // xtensa pretends to be arm

const zeroSizeAllocPtr uintptr = 16 // part of early flash: partition table, etc

// The bitness of the CPU (e.g. 8, 32, 64).
const TargetBits = 32

// Xtensa's windowed ABI uses a 17-word jmp_buf. JumpSP and JumpPC provide the
// first two words.
const deferExtraRegs = 15

const callInstSize = 3 // "callx0 someFunction" (and similar) is 3 bytes

// The largest alignment according to the Xtensa ABI is 8 (long long, double).
func align(ptr uintptr) uintptr {
	return (ptr + 7) &^ 7
}

func getCurrentStackPointer() uintptr {
	return uintptr(stacksave())
}

// Disable interrupts for procPin/procUnpin using the Xtensa RSIL/WSR PS
// instructions. There is one slot per core, because each core has its own PS.
var procPinnedMask [procPinSlots]uintptr

//go:linkname procPin sync/atomic.runtime_procPin
func procPin() {
	// rsil sets PS.INTLEVEL=15 (mask all) and returns the old PS.
	mask := uintptr(device.AsmFull("rsil {}, 15", nil))
	procPinnedMask[procPinSlot()] = mask
}

//go:linkname procUnpin sync/atomic.runtime_procUnpin
func procUnpin() {
	device.AsmFull("wsr {state}, PS", map[string]interface{}{
		"state": procPinnedMask[procPinSlot()],
	})
}
