//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"device/esp/radio/tsf"
	"runtime/interrupt"
	_ "unsafe" // for go:linkname
)

// How the TSF is read
//
// ESP-IDF v5.3.2 esp_wifi_get_tsf_time (esp_wifi.h) is in libnet80211.a
// (ieee80211_api.o), but it is slow for a clock. Outside of an interrupt it
// allocates a 24-byte message with _wifi_zalloc of the OS table, takes
// wifi_api_lock and posts the message to the Wi-Fi task with
// ieee80211_ioctl, then waits for wifi_get_tsf_time_process to run there.
// In an interrupt (_is_from_isr) it calls wifi_get_tsf_time_process itself,
// which checks the mode and the connection and calls hal_get_tsf_time
// (libpp.a, hal_mac.o).
//
// TSF calls hal_get_tsf_time directly, as the interrupt path does, so a read
// allocates nothing and does not wait for the Wi-Fi task. The disassembly of
// hal_get_tsf_time(0) shows this read of TSF0, the TSF of the station:
//
//	0x6003500C |= 1     latch TSF0 (bit 1 latches TSF1, for the soft-AP)
//	hi = 0x6003501C     bits 63..32
//	lo = 0x60035018     bits 31..0
//	0x6003500C &^= 1
//
// The Wi-Fi MAC registers are not in the ESP32-S3 Technical Reference Manual,
// so the blob is the only source for these addresses. The station blob
// enables TSF0 in wifi_station_start (hal_enable_sta_tsf) and disables it in
// wifi_station_stop. sta_recv_mgmt and sta_recv_assoc (ieee80211_sta.o) load
// the timestamp of the access point into it through ic_update_sta_tsf and
// hal_set_sta_tsf, which write 0x60035010 and 0x60035014 and set bit 4 of
// 0x6003500C.
//
// Before the connection, TSF0 counts from the station start and is not the
// TSF of the access point. So TSF gives ErrNotConnected outside of
// EventStaConnected and EventStaDisconnected (staConnected), and when the
// value is 0, as esp_wifi_get_tsf_time does before the first beacon.

// TSFReading is a TSF value with the local clock before and after it, see
// ReadTSF.
type TSFReading = tsf.Reading

//go:linkname nanotime runtime.nanotime
func nanotime() int64

// TSF gives the TSF (Timing Synchronization Function) timer of the station in
// microseconds, as esp_wifi_get_tsf_time(WIFI_IF_STA). All stations of one
// access point follow the TSF of its beacons, so they can use it as a common
// clock. It gives ErrNotConnected when the station is not connected.
//
// TSF does not allocate, take locks or wait, so any goroutine can call it
// often while Wi-Fi runs. It disables the interrupts of its core for the 4
// register accesses of the read. It must not run at the same time as Stop or
// Deinit, which stop the MAC.
//
// The latch register 0x6003500C is shared with the blobs, which write it from
// the Wi-Fi task and from the deferred interrupt on core 0. Because the
// goroutines of one core do not preempt each other, a read from a goroutine
// on core 0 (see machine.AssignCore) never overlaps a write of the blobs.
// From core 1, a write of the blobs can come during the read (a window of
// some hundred nanoseconds). Then a beacon update of the TSF can be lost
// until the next beacon, or the read latches a value that is newer by the
// same window.
//
// ESP-IDF notes that power save other than modem sleep can make the TSF
// inexact (esp_wifi.h). The PowerSave values of this package are all modem
// sleep, and this package does not use light sleep.
func TSF() (int64, error) {
	if !staConnected.Load() {
		return 0, ErrNotConnected
	}
	state := interrupt.Disable()
	t := int64(C.hal_get_tsf_time(ifStation))
	interrupt.Restore(state)
	if t == 0 {
		return 0, ErrNotConnected
	}
	return t, nil
}

// ReadTSF reads the TSF as TSF does, between two readings of the runtime
// clock (the monotonic clock of time.Now, TIMG0 timer 0 at 40 MHz, see
// src/runtime/runtime_esp32sx.go). The interrupts of the core stay disabled
// from the first clock reading to the last, so only the reads are in the
// window.
//
// The expected window (After - Before) is about 1 to 3 µs: each clock
// reading writes the update register of TIMG0 and waits for it, then reads 2
// registers, and the TSF read is 4 accesses to the MAC. An instruction cache
// miss in the window, or the other core in the clock lock (time.Now, sleep),
// makes it longer. The Local time of the reading is then within Window/2 of
// the true time of the TSF read, plus the 1 µs step of the TSF. These numbers
// are estimates: they are not measured on the board. ReadTSFBest removes the
// long windows.
//
// The TSF and TIMG0 both come from the 40 MHz crystal of the chip. So between
// beacon updates TSF - local changes only with the jumps of the updates,
// which follow the difference between the crystals of the access point and
// the board. A line fit of many readings (TSF against Local) gives the rate
// and the offset.
//
// See TSF for the errors, the locks and the cores.
func ReadTSF() (TSFReading, error) {
	if !staConnected.Load() {
		return TSFReading{}, ErrNotConnected
	}
	state := interrupt.Disable()
	before := nanotime()
	t := int64(C.hal_get_tsf_time(ifStation))
	after := nanotime()
	interrupt.Restore(state)
	if t == 0 {
		return TSFReading{}, ErrNotConnected
	}
	return TSFReading{TSF: t, Before: before, After: after}, nil
}

// ReadTSFBest calls ReadTSF n times and gives the reading with the smallest
// window. The interrupts run between the readings. An n below 1 is 1. It
// stops at the first error.
func ReadTSFBest(n int) (TSFReading, error) {
	return tsf.Best(n, ReadTSF)
}
