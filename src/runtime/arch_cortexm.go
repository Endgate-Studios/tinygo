//go:build cortexm

package runtime

import (
	"device/arm"
)

const GOARCH = "arm"

// The bitness of the CPU (e.g. 8, 32, 64).
const TargetBits = 32

const zeroSizeAllocPtr uintptr = 16 // part of the interrupt vector

const deferExtraRegs = 0

const callInstSize = 4 // "bl someFunction" is 4 bytes

// Align on word boundary.
func align(ptr uintptr) uintptr {
	return (ptr + 7) &^ 7
}

func getCurrentStackPointer() uintptr {
	return uintptr(stacksave())
}

// The safest thing to do here would just be to disable interrupts for
// procPin/procUnpin. There is one slot per core, because interrupts are
// disabled only on the current core.

var procPinnedMask [procPinSlots]uintptr

//go:linkname procPin sync/atomic.runtime_procPin
func procPin() {
	mask := arm.DisableInterrupts()
	procPinnedMask[procPinSlot()] = mask
}

//go:linkname procUnpin sync/atomic.runtime_procUnpin
func procUnpin() {
	arm.EnableInterrupts(procPinnedMask[procPinSlot()])
}
