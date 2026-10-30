//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"machine"
	"runtime"
	"runtime/interrupt"
	"sync"
	"time"
	"unsafe"
)

// Tasks of the blobs are goroutines. A task handle is the internal task
// pointer of the goroutine, as _task_get_current_task returns it. See
// current_task.go and current_task_cores.go for currentTask.

//export tinygo_wifi_task_create_pinned_to_core
func osiTaskCreatePinnedToCore(fn unsafe.Pointer, name *byte, stack uint32, param unsafe.Pointer, prio uint32, handle unsafe.Pointer, core uint32) int32 {
	return osiTaskCreate(fn, name, stack, param, prio, handle)
}

//export tinygo_wifi_task_create
func osiTaskCreate(fn unsafe.Pointer, name *byte, stack uint32, param unsafe.Pointer, prio uint32, handle unsafe.Pointer) int32 {
	started := make(chan unsafe.Pointer)
	go func() {
		machine.AssignCore(0) // see core.go
		started <- currentTask()
		C.seam_call(fn, param)
	}()
	t := <-started
	if handle != nil {
		*(*unsafe.Pointer)(handle) = t
	}
	return pdTrue
}

// A task can only delete itself. The blobs do not delete other tasks.
//
//export tinygo_wifi_task_delete
func osiTaskDelete(handle unsafe.Pointer) {
	if handle == nil || handle == currentTask() {
		runtime.Goexit()
	}
}

//export tinygo_wifi_task_delay
func osiTaskDelay(ticks uint32) {
	time.Sleep(time.Duration(ticks) * time.Millisecond)
}

//export tinygo_wifi_task_ms_to_tick
func osiMsToTick(ms uint32) int32 {
	return int32(ms)
}

//export tinygo_wifi_task_get_current_task
func osiTaskGetCurrentTask() unsafe.Pointer {
	return currentTask()
}

//export tinygo_wifi_task_get_max_priority
func osiTaskGetMaxPriority() int32 {
	return 25
}

//export tinygo_wifi_task_yield_from_isr
func osiTaskYieldFromISR() {
}

//export tinygo_wifi_is_from_isr
func osiIsFromISR() bool {
	return interrupt.In()
}

// Spin locks. wifi_int_disable disables the interrupts and takes the lock,
// as portENTER_CRITICAL.

//export tinygo_wifi_spin_lock_create
func osiSpinLockCreate() unsafe.Pointer {
	return handles.add(&spinLock{})
}

//export tinygo_wifi_spin_lock_delete
func osiSpinLockDelete(h unsafe.Pointer) {
	handles.remove(h)
}

//export tinygo_wifi_int_disable
func osiIntDisable(h unsafe.Pointer) uint32 {
	if l, ok := handles.get(h).(*spinLock); ok {
		return uint32(l.acquire())
	}
	return uint32(interrupt.Disable())
}

//export tinygo_wifi_int_restore
func osiIntRestore(h unsafe.Pointer, state uint32) {
	if l, ok := handles.get(h).(*spinLock); ok {
		l.release(interrupt.State(state))
		return
	}
	interrupt.Restore(interrupt.State(state))
}

// Timers. The blobs own the timer struct (ETSTimer). The Go state of a timer
// is in a map by the address of that struct. The service goroutine calls the
// timers that are due, so a timer does not need its own goroutine.
type osiTimer struct {
	fn, arg unsafe.Pointer
	when    time.Duration // since boot
	period  time.Duration
	armed   bool
	seq     uint32 // changes when the timer is armed, disarmed or set
}

var timers struct {
	sync.Mutex
	m     map[unsafe.Pointer]*osiTimer
	armed []*osiTimer
}

// disarmTimers stops all timers when no radio runs. The blobs set the function
// of a static timer once, so each timer keeps it for the next start.
func disarmTimers() {
	timers.Lock()
	for len(timers.armed) > 0 {
		disarmLocked(timers.armed[0])
	}
	timers.Unlock()
}

func timerOf(p unsafe.Pointer) *osiTimer {
	if timers.m == nil {
		timers.m = map[unsafe.Pointer]*osiTimer{}
	}
	t := timers.m[p]
	if t == nil {
		t = &osiTimer{}
		timers.m[p] = t
	}
	return t
}

//export tinygo_wifi_timer_setfn
func osiTimerSetfn(p, fn, arg unsafe.Pointer) {
	timers.Lock()
	t := timerOf(p)
	t.fn, t.arg = fn, arg
	t.seq++
	timers.Unlock()
}

func armTimer(p unsafe.Pointer, d time.Duration, repeat bool) {
	timers.Lock()
	t := timerOf(p)
	t.when = time.Since(boot) + d
	t.seq++
	t.period = 0
	if repeat {
		t.period = d
	}
	if !t.armed {
		t.armed = true
		timers.armed = append(timers.armed, t)
	}
	timers.Unlock()
	wakeService() // the timer can be earlier than the wait of the service
}

// disarmLocked removes t from the armed list.
func disarmLocked(t *osiTimer) {
	t.seq++
	if !t.armed {
		return
	}
	t.armed = false
	for i, a := range timers.armed {
		if a == t {
			last := len(timers.armed) - 1
			timers.armed[i] = timers.armed[last]
			timers.armed[last] = nil
			timers.armed = timers.armed[:last]
			break
		}
	}
}

// nextTimer gives the time until the next armed timer is due, or max.
func nextTimer(max time.Duration) time.Duration {
	now := time.Since(boot)
	d := max
	timers.Lock()
	for _, t := range timers.armed {
		d = min(d, t.when-now)
	}
	timers.Unlock()
	return d
}

// runTimers calls the timers that are due, each one once per pass. A callback
// can arm or disarm timers, so it runs without the lock.
func runTimers() {
	now := time.Since(boot)
	timers.Lock()
	var due [16]struct {
		t   *osiTimer
		seq uint32
	}
	n := 0
	for i := 0; i < len(timers.armed) && n < len(due); {
		t := timers.armed[i]
		if t.when > now {
			i++
			continue
		}
		if t.period != 0 {
			t.when = max(t.when+t.period, now+1)
			i++
		} else {
			disarmLocked(t) // moves the last timer to index i
		}
		due[n].t, due[n].seq = t, t.seq
		n++
	}
	timers.Unlock()
	for _, d := range due[:n] {
		// An earlier callback of this pass can disarm or arm the timer again.
		timers.Lock()
		fn, arg, ok := d.t.fn, d.t.arg, d.t.seq == d.seq
		timers.Unlock()
		if !ok {
			continue
		}
		if fn != nil {
			C.seam_call(fn, arg)
		}
	}
}

//export tinygo_wifi_timer_arm
func osiTimerArm(p unsafe.Pointer, ms uint32, repeat bool) {
	armTimer(p, time.Duration(ms)*time.Millisecond, repeat)
}

//export tinygo_wifi_timer_arm_us
func osiTimerArmUs(p unsafe.Pointer, us uint32, repeat bool) {
	armTimer(p, time.Duration(us)*time.Microsecond, repeat)
}

//export tinygo_wifi_timer_disarm
func osiTimerDisarm(p unsafe.Pointer) {
	timers.Lock()
	if t := timers.m[p]; t != nil {
		disarmLocked(t)
	}
	timers.Unlock()
}

//export tinygo_wifi_timer_done
func osiTimerDone(p unsafe.Pointer) {
	timers.Lock()
	if t := timers.m[p]; t != nil {
		disarmLocked(t)
		delete(timers.m, p)
	}
	timers.Unlock()
}
