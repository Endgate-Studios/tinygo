//go:build gc.custom && scheduler.cores

package gcshim

import "sync/atomic"

// heapLock is a spinlock for the heap across cores. It cannot sleep, because the scheduler allocates.
// A spinning core takes the GC interrupt, so no deadlock occurs (src/runtime/gc_stack_cores.go).
type heapLock struct {
	state atomic.Uint32
}

func (l *heapLock) Lock() {
	for !l.state.CompareAndSwap(0, 1) {
	}
}

func (l *heapLock) Unlock() {
	l.state.Store(0)
}
