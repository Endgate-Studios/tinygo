//go:build esp32s3

package radio

import (
	"machine"
	"time"
)

// servicePoll is the longest wait of the service goroutine. All work wakes
// the service, but with BLE coexistence HTTP throughput is about 18% lower with
// 100 ms. The cause is not known.
const servicePoll = 10 * time.Millisecond

var serviceStarted bool

// serviceWake marks new work for the service goroutine. A send that does not
// wait is safe in an interrupt, because the channel disables the interrupts.
var serviceWake = make(chan struct{}, 1)

func wakeService() {
	select {
	case serviceWake <- struct{}{}:
	default:
	}
}

// startService starts the one goroutine that does the deferred work of the
// blobs: the Wi-Fi interrupt, the blob timers and the PLL tracking of the PHY.
// It waits at most servicePoll. The interrupt handler, setLine and armTimer
// wake it through serviceWake.
func startService() {
	if serviceStarted {
		return
	}
	serviceStarted = true
	go func() {
		machine.AssignCore(0) // see core.go
		// The timer node is used again at each Reset, see resetTimer in
		// src/runtime/time.go, so the wait allocates nothing.
		wait := time.NewTimer(servicePoll)
		wait.Stop()
		for {
			applyLines()
			runISR()
			runTimers()
			runPLLTrack()
			poll := servicePoll
			if !wifiOn && !ble.enabled.Load() {
				poll = time.Hour // no radio runs, so only a wake brings work
			}
			d := nextTimer(poll)
			if d <= 0 {
				continue
			}
			wait.Reset(d)
			select {
			case <-serviceWake:
				// A failed Stop means that the timer fired, maybe on the other
				// core. Take its value, or the next wait ends at once.
				if !wait.Stop() {
					<-wait.C
				}
			case <-wait.C:
			}
		}
	}()
}
