package memory

import (
	"testing"
	"unsafe"
)

// fake is a bump allocator over buf that records its calls.
type fake struct {
	buf   [256]uint64
	off   uintptr
	calls int
	size  uintptr
	align uintptr
	freed []unsafe.Pointer
}

func (f *fake) Alloc(size, align uintptr) unsafe.Pointer {
	f.calls++
	f.size, f.align = size, align
	if f.off+size > unsafe.Sizeof(f.buf) {
		return nil
	}
	p := unsafe.Add(unsafe.Pointer(&f.buf[0]), f.off)
	f.off += (size + 7) &^ 7
	return p
}

func (f *fake) Free(p unsafe.Pointer) { f.freed = append(f.freed, p) }

type rec struct {
	a uint8
	b uint64
}

func TestNewAsksForSizeAndAlign(t *testing.T) {
	f := &fake{}
	p := New[rec](f)
	if p == nil {
		t.Fatal("New = nil")
	}
	if f.size != unsafe.Sizeof(rec{}) || f.align != unsafe.Alignof(rec{}) {
		t.Errorf("Alloc(%d, %d), want Alloc(%d, %d)", f.size, f.align, unsafe.Sizeof(rec{}), unsafe.Alignof(rec{}))
	}
}

func TestNewNilWhenFull(t *testing.T) {
	f := &fake{off: unsafe.Sizeof(fake{}.buf)}
	if p := New[rec](f); p != nil {
		t.Errorf("New on a full allocator = %p, want nil", p)
	}
}

func TestMakeSlice(t *testing.T) {
	f := &fake{}
	s := MakeSlice[uint32](f, 10)
	if len(s) != 10 || cap(s) != 10 {
		t.Fatalf("len %d cap %d, want 10 and 10", len(s), cap(s))
	}
	if f.size != 40 || f.align != unsafe.Alignof(uint32(0)) {
		t.Errorf("Alloc(%d, %d), want Alloc(40, %d)", f.size, f.align, unsafe.Alignof(uint32(0)))
	}
}

func TestMakeSliceZeroAndNegative(t *testing.T) {
	f := &fake{}
	if s := MakeSlice[uint32](f, 0); s == nil || len(s) != 0 {
		t.Errorf("MakeSlice(0) = %v, want an empty slice that is not nil", s)
	}
	if s := MakeSlice[uint32](f, -1); s != nil {
		t.Errorf("MakeSlice(-1) = %v, want nil", s)
	}
	if f.calls != 0 {
		t.Errorf("Alloc was called %d times, want 0", f.calls)
	}
}

func TestMakeSliceNilWhenFull(t *testing.T) {
	f := &fake{}
	if s := MakeSlice[uint64](f, 1000); s != nil {
		t.Errorf("MakeSlice larger than the allocator has len %d, want nil", len(s))
	}
}

// TestMakeSliceOverflow asks for n elements whose total size does not fit in a uintptr.
func TestMakeSliceOverflow(t *testing.T) {
	f := &fake{}
	n := int(^uintptr(0)/64 + 1)
	if s := MakeSlice[[64]byte](f, n); s != nil {
		t.Errorf("MakeSlice with an overflowing size has len %d, want nil", len(s))
	}
	if f.calls != 0 {
		t.Errorf("Alloc was called %d times, want 0", f.calls)
	}
}

func TestFreeUsesFreer(t *testing.T) {
	f := &fake{}
	p := New[rec](f)
	s := MakeSlice[uint32](f, 4)
	Free(f, p)
	FreeSlice(f, s[:2])
	if len(f.freed) != 2 || f.freed[0] != unsafe.Pointer(p) || f.freed[1] != unsafe.Pointer(&s[0]) {
		t.Errorf("Free got %v, want [%p %p]", f.freed, p, &s[0])
	}
}

func TestFreeNilAndEmpty(t *testing.T) {
	f := &fake{}
	Free[rec](f, nil)
	FreeSlice[uint32](f, nil)
	FreeSlice(f, []uint32{})
	if len(f.freed) != 0 {
		t.Errorf("Freer got %d calls, want 0", len(f.freed))
	}
}
