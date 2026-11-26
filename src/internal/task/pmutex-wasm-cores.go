//go:build scheduler.cores && tinygo.wasm

package task

import "sync/atomic"

// PMutex is a runtime lock, taken in calls compiled as never suspending:
// it waits on the core instead of pausing the goroutine.
type PMutex struct {
	state uint32 // 0 free, 1 held, 2 held with a core waiting
}

func (m *PMutex) Lock() {
	if atomic.CompareAndSwapUint32(&m.state, 0, 1) {
		return
	}
	for atomic.SwapUint32(&m.state, 2) != 0 {
		spinLoopWait()
		atomicWait32(&m.state, 2, 100_000)
	}
}

func (m *PMutex) Unlock() {
	switch atomic.SwapUint32(&m.state, 0) {
	case 0:
		runtimeFatal("sync: unlock of unlocked Mutex")
	case 2:
		atomicNotify(&m.state, 1)
	}
}

//export tinygo_atomic_wait32
func atomicWait32(addr *uint32, expected uint32, timeout int64) int32

//export tinygo_atomic_notify
func atomicNotify(addr *uint32, count uint32) uint32
