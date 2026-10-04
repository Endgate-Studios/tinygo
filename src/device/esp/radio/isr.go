//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"device/esp"
	"runtime/interrupt"
	"runtime/volatile"
	"sync/atomic"
	"unsafe"
)

// wifiCPUInt is a level 1 CPU line that TinyGo dispatches to Go
// (level1CPUInts in src/runtime/interrupt/interrupt_esp32s3.go).
const wifiCPUInt = 12

// The Wi-Fi sources ETS_WIFI_MAC_INTR_SOURCE to ETS_WIFI_BB_INTR_SOURCE of
// ESP-IDF v5.3.2 soc/esp32s3/include/soc/interrupts.h.
const lastWifiSource = 3

// isr holds the blob interrupt handlers. The blob gives its own CPU line
// numbers, and all of them share wifiCPUInt.
var isr struct {
	lock     spinLock
	handlers [32]struct{ fn, arg unsafe.Pointer }
	lines    uint32 // blob lines with a routed source
	enabled  uint32 // blob lines that _ints_on enabled
	pending  atomic.Bool
	hooked   bool
}

// osiSetIntr routes a Wi-Fi source to wifiCPUInt on core 0. The map register
// of source n is at INTERRUPT_CORE0 + 4n (ESP32-S3 TRM, Interrupt Matrix).
//
//export tinygo_wifi_set_intr
func osiSetIntr(cpu int32, source, num uint32, prio int32) {
	if source > lastWifiSource || num >= 32 {
		println("wifi: set_intr: source", source, "line", num, "is not a Wi-Fi interrupt")
		return
	}
	reg := (*volatile.Register32)(unsafe.Add(unsafe.Pointer(esp.INTERRUPT_CORE0), 4*source))
	reg.Set(wifiCPUInt)
	isr.lines |= 1 << num
}

// ESP-IDF v5.3.2 clear_intr_wrapper of esp_wifi/esp32s3/esp_adapter.c is empty.
//
//export tinygo_wifi_clear_intr
func osiClearIntr(source, num uint32) {}

//export tinygo_wifi_set_isr
func osiSetISR(n int32, fn, arg unsafe.Pointer) {
	if n < 0 || n >= 32 {
		return
	}
	state := isr.lock.acquire()
	isr.handlers[n].fn, isr.handlers[n].arg = fn, arg
	isr.lock.release(state)
}

//export tinygo_wifi_ints_on
func osiIntsOn(mask uint32) {
	state := isr.lock.acquire()
	isr.enabled |= mask & isr.lines
	on := isr.enabled != 0
	hook := on && !isr.hooked
	isr.hooked = isr.hooked || on
	isr.lock.release(state)
	if hook {
		interrupt.New(wifiCPUInt, wifiInterrupt).Enable()
	}
	if on {
		unmaskWifi()
	}
}

//export tinygo_wifi_ints_off
func osiIntsOff(mask uint32) {
	state := isr.lock.acquire()
	isr.enabled &^= mask
	off := isr.enabled == 0
	isr.lock.release(state)
	if off {
		maskWifi()
	}
}

// wifiInterrupt masks the line and leaves the work to runISR. The blob
// handlers are too deep for the stack of the interrupted goroutine.
func wifiInterrupt(interrupt.Interrupt) {
	C.seam_intenable_clear(1 << wifiCPUInt)
	isr.pending.Store(true)
	wakeService()
}

// runISR calls the blob handlers outside of the interrupt, then unmasks the
// line. The line is level triggered, so an interrupt that stays active comes
// back at once.
func runISR() {
	if !isr.pending.Swap(false) {
		return
	}
	state := isr.lock.acquire()
	handlers, lines := isr.handlers, isr.enabled
	isr.lock.release(state)
	for n := range handlers {
		if lines&(1<<n) != 0 && handlers[n].fn != nil {
			C.seam_call(handlers[n].fn, handlers[n].arg)
		}
	}
	if lines != 0 {
		unmaskWifi()
	}
}

func maskWifi()   { setLine(1<<wifiCPUInt, false) }
func unmaskWifi() { setLine(1<<wifiCPUInt, true) }

// osiSlowClockCal is esp_coex_common_clk_slowclk_cal_get_wrapper of ESP-IDF
// v5.3.2 esp_coex/esp32s3/esp_coex_adapter.c. It gives the 12-bit Wi-Fi
// format of the 19-bit period in RTC_SLOW_CLK_CAL_REG (RTC_CNTL_STORE1).
//
//export tinygo_wifi_slowclk_cal_get
func osiSlowClockCal() uint32 {
	cal := esp.RTC_CNTL.STORE1.Get()
	if cal == 0 {
		// TinyGo does not calibrate. Use the nominal 136 kHz RC_SLOW period.
		cal = uint32((1_000_000 << 19) / 136_000)
	}
	return cal >> (19 - 12)
}
