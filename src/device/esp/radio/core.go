//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"device"
	"runtime/interrupt"
	"sync/atomic"
)

// The radio sources go to CPU lines of core 0 (INTERRUPT_CORE0), and
// INTENABLE is a register of each core. So the blob tasks and the service
// goroutine stay on core 0, as the Wi-Fi task and the BT controller task in
// ESP-IDF (CONFIG_ESP_WIFI_TASK_PINNED_TO_CORE_0, controller_task_run_cpu).
// Code on core 1 gives its line changes to the service goroutine.

// coreID is 0 on core 0 and 1 on core 1, see ESP-IDF xt_utils_get_core_id.
func coreID() uint32 {
	return uint32(device.AsmFull("rsr {}, PRID", nil)) >> 13 & 1
}

var lines struct {
	want  atomic.Uint32 // the CPU lines that must be enabled on core 0
	dirty atomic.Uint32 // the lines that core 0 must update
}

// setLine enables or disables CPU lines of core 0. The wanted state is kept
// first, so a late change from core 1 cannot undo a newer one.
func setLine(mask uint32, on bool) {
	for {
		old := lines.want.Load()
		v := old &^ mask
		if on {
			v |= mask
		}
		if lines.want.CompareAndSwap(old, v) {
			break
		}
	}
	lines.dirty.Or(mask)
	if coreID() == 0 {
		applyLines()
	} else {
		wakeService()
	}
}

// applyLines writes the wanted state of the changed lines on core 0.
func applyLines() {
	state := interrupt.Disable()
	d := lines.dirty.Swap(0)
	want := lines.want.Load()
	C.seam_intenable_set(C.uint32_t(d & want))
	C.seam_intenable_clear(C.uint32_t(d &^ want))
	interrupt.Restore(state)
}
