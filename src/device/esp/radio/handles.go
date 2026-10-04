//go:build esp32s3

package radio

import (
	"runtime/interrupt"
	"sync/atomic"
	"unsafe"
)

// The blobs get a handle for each Go object, not a Go pointer. A handle is
// the index in the table plus one, so the GC never has to find Go objects
// through the memory of the blobs.
//
// The table does not allocate while it holds its lock, because the lock
// disables the interrupts. The free slots are a list inside the table.
type handleTable struct {
	lock      spinLock
	objs      []any
	firstFree int32 // index of the first free slot, or -1
}

// freeSlot is a free slot of the table, with the index of the next one.
type freeSlot int32

var handles = handleTable{firstFree: -1}

func (t *handleTable) add(o any) unsafe.Pointer {
	for {
		state := t.lock.acquire()
		if i := t.firstFree; i >= 0 {
			t.firstFree = int32(t.objs[i].(freeSlot))
			t.objs[i] = o
			t.lock.release(state)
			return unsafe.Pointer(uintptr(i + 1))
		}
		if n := len(t.objs); n < cap(t.objs) {
			t.objs = t.objs[:n+1]
			t.objs[n] = o
			t.lock.release(state)
			return unsafe.Pointer(uintptr(n + 1))
		}
		n := cap(t.objs)
		t.lock.release(state)
		bigger := make([]any, 0, 2*n+16)
		state = t.lock.acquire()
		if cap(t.objs) == n {
			t.objs = append(bigger, t.objs...)
		}
		t.lock.release(state)
	}
}

func (t *handleTable) get(h unsafe.Pointer) any {
	state := t.lock.acquire()
	var o any
	if i := uintptr(h) - 1; i < uintptr(len(t.objs)) {
		if _, free := t.objs[i].(freeSlot); !free {
			o = t.objs[i]
		}
	}
	// No defer: the Bluetooth interrupt calls get, and TinyGo has no defer
	// in an interrupt.
	t.lock.release(state)
	return o
}

func (t *handleTable) remove(h unsafe.Pointer) {
	state := t.lock.acquire()
	defer t.lock.release(state)
	i := uintptr(h) - 1
	if i >= uintptr(len(t.objs)) {
		return
	}
	if _, free := t.objs[i].(freeSlot); !free {
		t.objs[i] = freeSlot(t.firstFree)
		t.firstFree = int32(i)
	}
}

// spinLock is a lock that also works in an interrupt and across cores.
type spinLock struct {
	v atomic.Uint32
}

func (l *spinLock) acquire() interrupt.State {
	state := interrupt.Disable()
	for !l.v.CompareAndSwap(0, 1) {
	}
	return state
}

func (l *spinLock) release(state interrupt.State) {
	l.v.Store(0)
	interrupt.Restore(state)
}
