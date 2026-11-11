//go:build bcm2710

package runtime

import (
	"device/arm64"
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
	machine.InitSerial()
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

func sleepTicks(d timeUnit) {
	end := ticks() + d
	for ticks() < end {
		arm64.Asm("yield")
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
