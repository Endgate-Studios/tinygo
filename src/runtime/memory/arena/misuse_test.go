package arena

import (
	"testing"
	"unsafe"
)

// These tests show what a misuse does. Go has no check for them, so each one passes
// when the misuse gives wrong data with no error, or poison when it is on.

func fill(p unsafe.Pointer, n uintptr, b byte) { fillBytes(uintptr(p), n, b) }

func byteAt(p unsafe.Pointer, off uintptr) byte { return *(*byte)(unsafe.Add(p, off)) }

func TestUseAfterFreeReadsNextOwner(t *testing.T) {
	h := newTestHeap(t, 4<<10, WithPoison(false))
	p := h.Alloc(64, 0)
	fill(p, 64, 0x11)
	h.Free(p)
	q := h.Alloc(64, 0)
	if q != p {
		t.Fatalf("Alloc after Free = %p, want the freed block %p", q, p)
	}
	fill(q, 64, 0x22)
	if got := byteAt(p, 32); got != 0x22 {
		t.Errorf("old pointer reads %#x, want the new owner's 0x22", got)
	}
}

func TestUseAfterFreeSeesPoison(t *testing.T) {
	h := newTestHeap(t, 4<<10, WithPoison(true))
	p := h.Alloc(64, 0)
	fill(p, 64, 0x11)
	h.Free(p)
	// The first two payload words of a free block hold its list links (pushFree in heap.go).
	for off := 2 * wordSize; off < 48; off++ {
		if got := byteAt(p, off); got != poisonByte {
			t.Fatalf("byte %d after Free = %#x, want poison %#x", off, got, poisonByte)
		}
	}
}

func TestUseAfterResetReadsNextOwner(t *testing.T) {
	r := newTestRegion(t, make([]byte, 4<<10), WithRegionPoison(false))
	p := r.Alloc(64, 0)
	fill(p, 64, 0x11)
	r.Reset()
	q := r.Alloc(64, 0)
	if q != p {
		t.Fatalf("Alloc after Reset = %p, want %p", q, p)
	}
	fill(q, 64, 0x22)
	if got := byteAt(p, 32); got != 0x22 {
		t.Errorf("old pointer reads %#x, want the new owner's 0x22", got)
	}
}

func TestUseAfterResetSeesPoison(t *testing.T) {
	r := newTestRegion(t, make([]byte, 4<<10), WithRegionPoison(true))
	p := r.Alloc(64, 0)
	fill(p, 64, 0x11)
	r.Reset()
	for off := uintptr(0); off < 64; off++ {
		if got := byteAt(p, off); got != poisonByte {
			t.Fatalf("byte %d after Reset = %#x, want poison %#x", off, got, poisonByte)
		}
	}
}

func TestOldPointerAfterReallocMove(t *testing.T) {
	h := newTestHeap(t, 8<<10, WithPoison(false))
	p := h.Alloc(32, 0)
	fill(p, 32, 0x11)
	h.Alloc(32, 0) // a used block after p, so that p cannot grow in place
	q := h.Realloc(p, 2048)
	if q == nil || q == p {
		t.Fatalf("Realloc = %p, want a new block", q)
	}
	r := h.Alloc(32, 0)
	if r != p {
		t.Fatalf("Alloc after the move = %p, want the old block %p", r, p)
	}
	fill(r, 32, 0x33)
	if got := byteAt(p, 16); got != 0x33 {
		t.Errorf("old pointer reads %#x, want the new owner's 0x33", got)
	}
	if got := byteAt(q, 16); got != 0x11 {
		t.Errorf("new pointer reads %#x, want the moved 0x11", got)
	}
}
