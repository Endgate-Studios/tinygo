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
	"unsafe"
)

// btCPUInt is a second level 1 CPU line that TinyGo dispatches to Go
// (level1CPUInts in src/runtime/interrupt/interrupt_esp32s3.go).
const btCPUInt = 13

// The controller sources ETS_BT_BB_INTR_SOURCE and ETS_RWBLE_INTR_SOURCE of
// ESP-IDF v5.3.2 soc/esp32s3/include/soc/interrupts.h.
const (
	sourceBTBB  = 5
	sourceRWBLE = 8
)

type btHandler struct {
	source  int32
	fn, arg unsafe.Pointer
	enabled bool
}

// btIsr holds the handlers of _interrupt_alloc. A handle is a pointer to one
// entry. The handlers run in the interrupt, as with ESP-IDF, because the
// controller misses its radio events when they wait for the service.
var btIsr struct {
	lock   spinLock
	h      [2]btHandler
	hooked bool
}

// btInterruptAlloc is interrupt_alloc_wrapper of bt.c without esp_intr_alloc:
// it routes the source to btCPUInt on core 0 and keeps the handler.
//
//export tinygo_bt_interrupt_alloc
func btInterruptAlloc(cpu, source int32, fn, arg unsafe.Pointer, handle *unsafe.Pointer) int32 {
	if source != sourceBTBB && source != sourceRWBLE {
		return -1
	}
	state := btIsr.lock.acquire()
	slot := -1
	for i := range btIsr.h {
		if btIsr.h[i].fn == nil {
			slot = i
			break
		}
	}
	if slot >= 0 {
		btIsr.h[slot] = btHandler{source: source, fn: fn, arg: arg, enabled: true}
	}
	hook := slot >= 0 && !btIsr.hooked
	btIsr.hooked = btIsr.hooked || hook
	btIsr.lock.release(state)
	if slot < 0 {
		return -1
	}
	// The map register of source n is at INTERRUPT_CORE0 + 4n (ESP32-S3
	// TRM, Interrupt Matrix).
	reg := (*volatile.Register32)(unsafe.Add(unsafe.Pointer(esp.INTERRUPT_CORE0), 4*source))
	reg.Set(btCPUInt)
	if hook {
		interrupt.New(btCPUInt, btInterrupt).Enable()
	}
	*handle = unsafe.Pointer(&btIsr.h[slot])
	unmaskBT()
	return 0
}

//export tinygo_bt_interrupt_free
func btInterruptFree(handle unsafe.Pointer) int32 {
	state := btIsr.lock.acquire()
	*(*btHandler)(handle) = btHandler{}
	btIsr.lock.release(state)
	return 0
}

//export tinygo_bt_interrupt_enable
func btInterruptEnable(handle unsafe.Pointer) int32 {
	state := btIsr.lock.acquire()
	(*btHandler)(handle).enabled = true
	btIsr.lock.release(state)
	unmaskBT()
	return 0
}

//export tinygo_bt_interrupt_disable
func btInterruptDisable(handle unsafe.Pointer) int32 {
	state := btIsr.lock.acquire()
	(*btHandler)(handle).enabled = false
	btIsr.lock.release(state)
	return 0
}

// btInterrupt calls the enabled handlers. They only use the _from_isr
// functions, and those do not block.
func btInterrupt(interrupt.Interrupt) {
	for i := range btIsr.h {
		if h := &btIsr.h[i]; h.fn != nil && h.enabled {
			C.seam_call(h.fn, h.arg)
		}
	}
}

func unmaskBT() { setLine(1<<btCPUInt, true) }
