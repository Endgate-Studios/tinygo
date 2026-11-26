//go:build esp32s3

package radio

import (
	"machine"
	"unsafe"
)

// userException replaces the weak espradio_user_exception of
// targets/esp32s3-interrupts.S. It prints the exception and stops. See the
// Xtensa ISA Reference Manual, table 4-64 for the EXCCAUSE codes.
//
//export espradio_user_exception
func userException(cause, epc, vaddr uint32, frame unsafe.Pointer) {
	// println can wait forever for the print lock that the interrupted code
	// holds, so write to the UART directly.
	excPrint("wifi: exception core", coreID())
	excPrint(" cause", cause)
	excPrint(" pc", epc)
	excPrint(" address", vaddr)
	excPrint("\n", 0)
	// The frame of targets/esp32s3-interrupts.S: a0 to a15, then SAR, EPC1,
	// PS, WINDOWBASE and WINDOWSTART.
	for i, v := range (*[21]uint32)(frame) {
		excPrint("  frame", uint32(i))
		excPrint(" ", v)
		excPrint("\n", 0)
	}
	for {
	}
}

// excPrint writes s, then v as hexadecimal unless s ends in a newline.
func excPrint(s string, v uint32) {
	for i := 0; i < len(s); i++ {
		machine.Serial.WriteByte(s[i])
	}
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return
	}
	machine.Serial.WriteByte(' ')
	for shift := 28; shift >= 0; shift -= 4 {
		machine.Serial.WriteByte("0123456789abcdef"[v>>uint(shift)&0xf])
	}
}
