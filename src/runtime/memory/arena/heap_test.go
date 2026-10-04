package arena

import (
	"bytes"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"unsafe"

	"runtime/memory"
)

// The compiler checks that the strategies implement the shared interfaces.
var (
	_ memory.Allocator = (*Heap)(nil)
	_ memory.Freer     = (*Heap)(nil)
	_ memory.Allocator = (*Region)(nil)
	_ memory.Resetter  = (*Region)(nil)
)

func newTestHeap(t *testing.T, size int, opts ...HeapOption) *Heap {
	t.Helper()
	h, err := NewHeap(make([]byte, size), opts...)
	if err != nil {
		t.Fatalf("NewHeap: %v", err)
	}
	return h
}

func mustCheck(t *testing.T, h *Heap) {
	t.Helper()
	if err := h.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestNewHeap(t *testing.T) {
	tests := []struct {
		name    string
		mem     []byte
		opts    []HeapOption
		wantErr bool
	}{
		{name: "empty", mem: nil, wantErr: true},
		{name: "too small", mem: make([]byte, 8), wantErr: true},
		{name: "nil locker", mem: make([]byte, 1024), opts: []HeapOption{WithLocker(nil)}, wantErr: true},
		{name: "ok", mem: make([]byte, 1024)},
		{name: "ok with locker", mem: make([]byte, 1024), opts: []HeapOption{WithLocker(&sync.Mutex{})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewHeap(tt.mem, tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewHeap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				mustCheck(t, h)
			}
		})
	}
	if _, err := NewHeap(make([]byte, 8)); !errors.Is(err, ErrArenaTooSmall) {
		t.Errorf("NewHeap(8 bytes) error = %v, want ErrArenaTooSmall", err)
	}
}

func TestHeapAllocAlignment(t *testing.T) {
	h := newTestHeap(t, 64<<10)
	for _, align := range []uintptr{0, 1, 8, 16, 32, 64, 256, 4096} {
		for _, size := range []uintptr{1, 7, 24, 100, 1000} {
			p := h.Alloc(size, align)
			if p == nil {
				t.Fatalf("Alloc(%d, %d) = nil", size, align)
			}
			want := max(align, DefaultAlign)
			if uintptr(p)%want != 0 {
				t.Errorf("Alloc(%d, %d) = %p, not %d-aligned", size, align, p, want)
			}
			mustCheck(t, h)
		}
	}
	if h.Alloc(8, 3) != nil {
		t.Error("Alloc with non-power-of-two alignment != nil")
	}
}

func TestHeapFreeCoalescesBackToOneBlock(t *testing.T) {
	h := newTestHeap(t, 16<<10)
	initial := h.Stats()
	var ps []unsafe.Pointer
	for i := 0; i < 20; i++ {
		ps = append(ps, h.Alloc(uintptr(16+i*8), 0))
	}
	// This order makes Free join blocks on both sides.
	for _, i := range []int{1, 3, 2, 0, 19, 10, 12, 11, 4, 5, 6, 7, 8, 9, 13, 14, 15, 16, 17, 18} {
		h.Free(ps[i])
		mustCheck(t, h)
	}
	got := h.Stats()
	if got.FreeBlocks != 1 || got.LargestFree != initial.LargestFree || got.InUse != 0 {
		t.Errorf("after freeing everything: %+v, want one free block of %d", got, initial.LargestFree)
	}
}

func TestHeapFreePanics(t *testing.T) {
	h := newTestHeap(t, 4096)
	p := h.Alloc(32, 0)
	var local [4]uintptr

	tests := []struct {
		name string
		free func()
	}{
		{name: "double free", free: func() { h.Free(p); h.Free(p) }},
		{name: "foreign pointer", free: func() { h.Free(unsafe.Pointer(&local[2])) }},
		{name: "interior pointer", free: func() { h.Free(unsafe.Add(h.Alloc(64, 0), 8)) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			tt.free()
		})
	}
}

func TestHeapReallocPreservesAndZeroes(t *testing.T) {
	h := newTestHeap(t, 64<<10)
	p := h.Alloc(40, 0)
	fillBytes(uintptr(p), 40, 0x5A)
	neighbour := h.Alloc(40, 0)

	tests := []struct {
		name string
		size uintptr
	}{
		{name: "shrink", size: 16},
		{name: "grow within block", size: 40},
		{name: "grow by moving", size: 500},
		{name: "grow in place into freed neighbour", size: 2000},
	}
	kept := uintptr(16)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "grow in place into freed neighbour" {
				h.Free(neighbour)
			}
			q := h.Realloc(p, tt.size)
			if q == nil {
				t.Fatal("Realloc = nil")
			}
			got := bytesAt(uintptr(q), tt.size)
			if !bytes.Equal(got[:kept], bytes.Repeat([]byte{0x5A}, int(kept))) {
				t.Fatalf("prefix lost: % x", got[:kept])
			}
			for i, b := range got[kept:] {
				if b != 0 {
					t.Fatalf("byte %d past old size = %#x, want 0", int(kept)+i, b)
				}
			}
			p = q
			mustCheck(t, h)
		})
	}
	if h.Realloc(p, 0) != nil {
		t.Error("Realloc(p, 0) != nil")
	}
	mustCheck(t, h)
}

func TestHeapExhaustion(t *testing.T) {
	h := newTestHeap(t, 4096)
	if h.Alloc(8192, 0) != nil {
		t.Fatal("oversized Alloc != nil")
	}
	if h.Calloc(^uintptr(0)/2, 4) != nil {
		t.Fatal("overflowing Calloc != nil")
	}
	if got := h.Stats().Failures; got != 2 {
		t.Errorf("Failures = %d, want 2", got)
	}
}

func TestHeapPoison(t *testing.T) {
	h := newTestHeap(t, 4096, WithPoison(true))
	p := h.Alloc(64, 0)
	h.Alloc(64, 0) // stops p from joining the tail block
	h.Free(p)
	// The first words hold free list links. The rest must hold poison.
	if b := *(*byte)(unsafe.Add(p, 3*wordSize)); b != poisonByte {
		t.Errorf("freed byte = %#x, want %#x", b, poisonByte)
	}
}

// FuzzHeap runs random Alloc, Free and Realloc calls and checks the heap at each step.
// A model records the contents of each live allocation.
func FuzzHeap(f *testing.F) {
	f.Add(int64(1), uint16(500))
	f.Add(int64(42), uint16(2000))
	f.Fuzz(func(t *testing.T, seed int64, steps uint16) {
		h := newTestHeap(t, 32<<10)
		rng := rand.New(rand.NewSource(seed))
		type live struct {
			p    unsafe.Pointer
			size uintptr
			tag  byte
		}
		var objs []live
		verify := func(o live) {
			for i, b := range bytesAt(uintptr(o.p), o.size) {
				if b != o.tag {
					t.Fatalf("object %p byte %d = %#x, want %#x", o.p, i, b, o.tag)
				}
			}
		}
		for step := 0; step < int(steps); step++ {
			switch op := rng.Intn(4); {
			case op <= 1 || len(objs) == 0:
				size := uintptr(1 + rng.Intn(700))
				align := uintptr(1) << rng.Intn(8)
				p := h.Alloc(size, align)
				if p == nil {
					continue
				}
				o := live{p: p, size: size, tag: byte(rng.Intn(255) + 1)}
				fillBytes(uintptr(p), size, o.tag)
				objs = append(objs, o)
			case op == 2:
				i := rng.Intn(len(objs))
				verify(objs[i])
				h.Free(objs[i].p)
				objs = append(objs[:i], objs[i+1:]...)
			default:
				i := rng.Intn(len(objs))
				o := objs[i]
				size := uintptr(1 + rng.Intn(1200))
				q := h.Realloc(o.p, size)
				if q == nil {
					verify(o)
					continue
				}
				kept := min(o.size, size)
				verify(live{p: q, size: kept, tag: o.tag})
				fillBytes(uintptr(q), size, o.tag)
				objs[i] = live{p: q, size: size, tag: o.tag}
			}
			if err := h.Check(); err != nil {
				t.Fatalf("step %d: %v", step, err)
			}
		}
		for _, o := range objs {
			verify(o)
		}
	})
}

func TestRegion(t *testing.T) {
	r, err := NewRegion(make([]byte, 1024))
	if err != nil {
		t.Fatal(err)
	}
	a := r.Alloc(10, 0)
	b := r.Alloc(10, 64)
	if a == nil || b == nil || uintptr(b)%64 != 0 || uintptr(a)%DefaultAlign != 0 {
		t.Fatalf("Alloc = %p, %p", a, b)
	}
	m := r.Mark()
	r.Alloc(100, 0)
	r.Rewind(m)
	if got := r.Mark(); got != m {
		t.Errorf("after Rewind Mark = %d, want %d", got, m)
	}
	if r.Alloc(2000, 0) != nil {
		t.Error("oversized Alloc != nil")
	}
	fillBytes(uintptr(a), 10, 0xFF)
	r.Reset()
	if c := r.Alloc(10, 0); c != a || *(*byte)(c) != 0 {
		t.Errorf("after Reset Alloc = %p (want %p) first byte %#x (want 0)", c, a, *(*byte)(c))
	}
	s := r.Stats()
	if s.Resets != 1 || s.Failures != 1 || s.Peak < 100 {
		t.Errorf("Stats = %+v", s)
	}
}

func TestTypedHelpersOverArena(t *testing.T) {
	type point struct{ X, Y int64 }
	h := newTestHeap(t, 4096)
	p := memory.New[point](h)
	if p == nil || *p != (point{}) {
		t.Fatalf("New = %v", p)
	}
	s := memory.MakeSlice[point](h, 10)
	if len(s) != 10 || cap(s) != 10 {
		t.Fatalf("MakeSlice len=%d cap=%d", len(s), cap(s))
	}
	memory.Free(h, p)
	memory.FreeSlice(h, s)
	mustCheck(t, h)
	if h.Stats().InUse != 0 {
		t.Error("typed frees left memory in use")
	}
}

func TestHeapGrow(t *testing.T) {
	h := newTestHeap(t, 1024)
	small := h.Stats().Size
	if p := h.Alloc(2048, 0); p != nil {
		t.Fatal("a 2 KiB block fits in a 1 KiB heap")
	}
	if err := h.Grow(make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	mustCheck(t, h)
	if s := h.Stats(); s.Size <= small || s.FreeBlocks != 2 {
		t.Fatalf("after Grow: %+v", s)
	}
	big := h.Alloc(2048, 0)
	little := h.Alloc(64, 0)
	if big == nil || little == nil {
		t.Fatal("Alloc failed after Grow")
	}
	mustCheck(t, h)

	// Realloc moves the block to the span that has room.
	*(*byte)(little) = 0x5a
	moved := h.Realloc(little, 1500)
	if moved == nil || *(*byte)(moved) != 0x5a {
		t.Fatal("Realloc across spans lost the data")
	}
	mustCheck(t, h)

	h.Free(big)
	h.Free(moved)
	mustCheck(t, h)
	if s := h.Stats(); s.InUse != 0 || s.FreeBlocks != 2 {
		t.Errorf("after freeing everything: %+v, want one free block in each span", s)
	}
}

func TestHeapGrowLimits(t *testing.T) {
	h := newTestHeap(t, 256)
	if err := h.Grow(nil); err != ErrArenaTooSmall {
		t.Errorf("Grow(nil) = %v, want %v", err, ErrArenaTooSmall)
	}
	for i := 1; i < MaxSpans; i++ {
		if err := h.Grow(make([]byte, 256)); err != nil {
			t.Fatalf("span %d: %v", i, err)
		}
	}
	if err := h.Grow(make([]byte, 256)); err != ErrTooManySpans {
		t.Errorf("span %d: %v, want %v", MaxSpans, err, ErrTooManySpans)
	}
	mustCheck(t, h)
}

func TestHeapContainsAndSize(t *testing.T) {
	h := newTestHeap(t, 1024)
	other := newTestHeap(t, 1024)
	p := h.Alloc(100, 0)
	if !h.Contains(p) || other.Contains(p) {
		t.Error("Contains is wrong")
	}
	if n := h.Size(p); n < 100 || n > 100+2*DefaultAlign {
		t.Errorf("Size = %d for 100 bytes", n)
	}
	if err := h.Grow(make([]byte, 512)); err != nil {
		t.Fatal(err)
	}
	q := h.Alloc(300, 0)
	if !h.Contains(q) {
		t.Error("Contains misses the second span")
	}
}
