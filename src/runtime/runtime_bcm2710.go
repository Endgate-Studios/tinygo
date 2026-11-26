//go:build bcm2710

package runtime

import (
	"device/arm64"
	"device/bcm2710"
	"machine"
	_ "machine/usb/cdc"
)

//export main
func main() {
	preinit()
	arm64.Asm("msr daifclr, #2")
	initPeripherals()
	run()
	exit(0)
}

// zeroSizeAllocPtr is in the armstub, which nothing reads after boot.
const zeroSizeAllocPtr uintptr = 16

// preinit has nothing to do: start.S zeroes .bss, and the firmware loaded .data in place.
func preinit() {}

func initPeripherals() {
	enableCoreInterrupts(0)
	machine.InitSerial()
}

// enableCoreInterrupts lets the core's physical timer and its mailbox 0 interrupt it.
func enableCoreInterrupts(core uint32) {
	bcm2710.LOCAL.TIMER_INT_CTL[core].Set(bcm2710.LOCAL_IRQ_CNTPNS)
	bcm2710.LOCAL.MAILBOX_INT_CTL[core].Set(1)
}

func putchar(c byte) {
	machine.Serial.WriteByte(c)
}

func getchar() byte {
	for machine.Serial.Buffered() == 0 {
		Gosched()
	}
	v, _ := machine.Serial.ReadByte()
	return v
}

func buffered() int {
	return machine.Serial.Buffered()
}

// The armstub sets the generic timer to the 19.2 MHz crystal (raspberrypi/tools armstubs/armstub8.S).
func ticksToNanoseconds(ticks timeUnit) int64 {
	return int64(ticks) * 625 / 12
}

func nanosecondsToTicks(ns int64) timeUnit {
	return timeUnit(ns * 12 / 625)
}

func ticks() timeUnit {
	return timeUnit(arm64.AsmFull("mrs {}, cntpct_el0", nil))
}

// setTimer starts this core's physical timer for the given time. Its interrupt stops it.
func setTimer(target timeUnit) {
	arm64.AsmFull("msr cntp_cval_el0, {t}", map[string]interface{}{"t": uintptr(target)})
	arm64.AsmFull("msr cntp_ctl_el0, {c}", map[string]interface{}{"c": uintptr(1)})
	arm64.Asm("isb")
}

// waitForInterrupt sleeps until an interrupt is pending and lets it run, with IRQs
// masked before and after so a wake isn't lost between a check and the wfi.
func waitForInterrupt() {
	arm64.Asm("wfi")
	arm64.Asm("msr daifclr, #2")
	arm64.Asm("isb")
	arm64.Asm("msr daifset, #2")
}

func sleepTicks(d timeUnit) {
	end := ticks() + d
	for ticks() < end {
		setTimer(end)
		arm64.Asm("wfi")
	}
}

func exit(code int) {
	abort()
}

func abort() {
	for {
		arm64.Asm("wfe")
	}
}

//export handleException
func handleException(kind, esr, elr, far uint64) {
	print("exception ", kind, " esr=")
	printhex(esr)
	print(" elr=")
	printhex(elr)
	print(" far=")
	printhex(far)
	println()
	abort()
}

func printhex(v uint64) {
	const digits = "0123456789abcdef"
	print("0x")
	for i := 60; i >= 0; i -= 4 {
		putchar(digits[(v>>uint(i))&0xf])
	}
}
