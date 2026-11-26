//go:build esp32s3 && scheduler.cores

package nn

import "sync/atomic"

// This is the esp-nn dual core support (esp_nn_multicore.c) with a worker
// goroutine. It is not pinned: while the caller runs on one core, the other
// core picks up the worker. esp-nn pins it because FreeRTOS tasks do not move.

var dual struct {
	jobs    chan func()
	done    chan struct{}
	busy    atomic.Bool
	scratch []byte
}

func enableDualCore() {
	if dual.jobs != nil {
		return
	}
	// Buffered, so that the caller does not stop and give its core to the
	// worker.
	dual.jobs = make(chan func(), 1)
	dual.done = make(chan struct{}, 1)
	go func() {
		for f := range dual.jobs {
			f()
			dual.done <- struct{}{}
		}
	}()
}

// dualReady reports if dualRun can start a job. Check it before making the
// job closure, so that there is no closure when the worker is off.
func dualReady() bool {
	return dual.jobs != nil && !dual.busy.Load()
}

// dualRun starts f on the other core with a 16 byte aligned scratch buffer of
// n bytes. If it returns true, the caller must call dualWait.
func dualRun(n int, f func(scratch []byte)) bool {
	if dual.jobs == nil || !dual.busy.CompareAndSwap(false, true) {
		return false
	}
	if len(dual.scratch) < n+16 {
		dual.scratch = make([]byte, n+16)
	}
	s := align16(dual.scratch)[:n]
	dual.jobs <- func() { f(s) }
	return true
}

func dualWait() {
	<-dual.done
	dual.busy.Store(false)
}
