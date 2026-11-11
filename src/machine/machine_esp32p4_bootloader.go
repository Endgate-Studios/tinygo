//go:build esp32p4

package machine

import (
	"device/esp"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

// The ROM's download mode on the next start, over UART0 or USB. RTC_CNTL_OPTION1_REG and its
// FORCE_DOWNLOAD_BOOT bit, in esptool's targets/esp32p4.py, which clears it again after flashing.
const (
	romOption1          = 0x50110008
	romForceDownloadBit = 0x4
)

//export esp_rom_software_reset_system
func romResetSystem()

// lpWatchdogKey unlocks the LP watchdog's registers (ESP-IDF hal/esp32p4/include/hal/lpwdt_ll.h).
const lpWatchdogKey = 0x50D83AA1

// EnterBootloader restarts the chip into the ROM's download mode, as a 1200 baud touch on USB asks. The LP watchdog goes off
// first: a restart leaves it going, nothing in the ROM feeds it, and a CrowPanel's flash takes about 18 s of its 20.
func EnterBootloader() {
	interrupt.Disable()
	esp.LP_WDT.WPROTECT.Set(lpWatchdogKey)
	esp.LP_WDT.CONFIG0.Set(0)
	esp.LP_WDT.WPROTECT.Set(0)
	usbLeaveBus()
	(*volatile.Register32)(unsafe.Pointer(uintptr(romOption1))).SetBits(romForceDownloadBit)
	romResetSystem()
	for {
	}
}
