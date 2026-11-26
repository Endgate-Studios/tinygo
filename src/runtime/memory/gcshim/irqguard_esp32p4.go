//go:build gc.custom && baremetal && esp32p4

package gcshim

import (
	"device/riscv"
	"unsafe"
)

// guardInterrupts, once GuardInterrupts has turned it on, makes a heap allocation with this core's interrupts off a crash. Such an
// allocation can wait on the heap lock while the other core collects, and the collector waits for this core to take its pause
// interrupt, which it can't: both cores stop for good.
var guardInterrupts bool

// GuardInterrupts turns the check on: after startup, as init allocates before interrupts are on.
func GuardInterrupts() { guardInterrupts = true }

// checkInterrupts crashes if this core's interrupts are off, first printing the code addresses on the stack, which say who allocated.
func checkInterrupts() {
	if !guardInterrupts || riscv.MSTATUS.Get()&riscv.MSTATUS_MIE != 0 {
		return
	}
	var here uintptr // its address is on the stack, at the top of what's above
	sp := uintptr(unsafe.Pointer(&here))
	println("heap alloc with interrupts off, from:")
	for i, n := uintptr(0), 0; i < 512 && n < 24; i++ {
		if v := *(*uintptr)(unsafe.Pointer(sp + i*4)); v >= 0x40000000 && v < 0x44000000 {
			print(" ", unsafe.Pointer(v))
			n++
		}
	}
	println()
	runtimePanic("heap alloc with interrupts off")
}
