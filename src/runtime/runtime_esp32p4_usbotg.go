//go:build esp32p4 && usbotg && serial.usb

package runtime

import (
	_ "machine/usb/cdc"
	"runtime/volatile"
	"unsafe"
)

// A crash can't print over USB, as that needs interrupts, so what's printed is kept in LP RAM, which a
// reset leaves alone, and machine.LastCrash has it after the restart. The layout is shared with that.
const (
	crashWords = 0x50108000 // SOC_LP_RAM_LOW in ESP-IDF soc/esp32p4/include/soc/soc.h
	crashFlag  = 0
	crashNext  = 1
	crashText  = 2
	crashSize  = 512
	crashMagic = 0x43524153
)

func crashWord(i uintptr) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(uintptr(crashWords) + i*4))
}

func crashKeep(c byte) {
	next := crashWord(crashNext).Get()
	crashWord(crashText + uintptr(next%crashSize)).Set(uint32(c))
	crashWord(crashNext).Set(next + 1)
}

func crashRestart() {
	crashWord(crashFlag).Set(crashMagic)
	resetWholeChip()
}
