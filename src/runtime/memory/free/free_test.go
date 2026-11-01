package free

import (
	"runtime"
	"testing"
	"unsafe"
)

var sink []byte

func frees() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Frees
}

func TestNilAndEmpty(t *testing.T) {
	Free[int](nil)
	FreeSlice[int](nil)
	FreeSlice(make([]int, 0))
	Runtime{}.Free(nil)
}

// TestVerifyMatchesIsReal fails for memory_realfree on a runtime whose free does nothing.
func TestVerifyMatchesIsReal(t *testing.T) {
	if Verify() != IsReal() {
		t.Errorf("Verify() = %v, IsReal() = %v", Verify(), IsReal())
	}
}

// TestNoOpWithoutRealFree is the regression test for upstream commit 02021b58. Since then,
// runtime.free of the built-in collectors stops the program for a GC object.
func TestNoOpWithoutRealFree(t *testing.T) {
	if IsReal() {
		t.Skip("this build has a real free")
	}
	p := new([8]uintptr)
	for i := range p {
		p[i] = uintptr(i)
	}
	s := make([]byte, 64)
	before := frees()
	Free(p)
	FreeSlice(s)
	Runtime{}.Free(unsafe.Pointer(p))
	if n := frees() - before; n != 0 {
		t.Errorf("MemStats.Frees grew by %d, want 0", n)
	}
	runtime.GC()
	for i, v := range p {
		if v != uintptr(i) {
			t.Fatalf("p[%d] = %d after a free that does nothing", i, v)
		}
	}
}

func TestFreeCountsInMemStats(t *testing.T) {
	if !IsReal() {
		t.Skip("this build has no real free")
	}
	p := new([8]uintptr)
	before := frees()
	Free(p)
	if n := frees() - before; n != 1 {
		t.Errorf("MemStats.Frees grew by %d, want 1", n)
	}
}

func TestFreeSliceReusesMemory(t *testing.T) {
	if !IsReal() {
		t.Skip("this build has no real free")
	}
	sink = make([]byte, 4096)
	first := unsafe.SliceData(sink)
	FreeSlice(sink)
	sink = make([]byte, 4096)
	if unsafe.SliceData(sink) != first {
		t.Errorf("the next allocation of the same size did not use the freed memory")
	}
	sink = nil
}

// TestFreeOutsideHeapDoesNothing frees pointers to a global and to the stack. gcshim
// ignores a pointer outside its heap (free in src/runtime/memory/gcshim/shim.go).
func TestFreeOutsideHeapDoesNothing(t *testing.T) {
	var local [8]uintptr
	before := frees()
	Free(&global)
	Free(&local)
	if n := frees() - before; n != 0 {
		t.Errorf("MemStats.Frees grew by %d, want 0", n)
	}
	if global[0] != 1 {
		t.Errorf("global changed to %d", global[0])
	}
}

var global = [8]uintptr{1}

type node struct {
	next *node
	val  int
}

// TestEarlyFreeKeepsLiveData frees many objects early, then checks that collections keep a
// list that was live the whole time.
func TestEarlyFreeKeepsLiveData(t *testing.T) {
	var head *node
	for i := range 500 {
		head = &node{next: head, val: i}
	}
	for i := range 1000 {
		b := make([]byte, 16+i%200)
		b[0] = byte(i)
		FreeSlice(b)
		Free(&node{val: i})
	}
	for range 3 {
		runtime.GC()
		for i := range 1000 {
			sink = make([]byte, 16+i%200)
		}
	}
	sink = nil
	n, sum := 0, 0
	for p := head; p != nil; p = p.next {
		n++
		sum += p.val
	}
	if n != 500 || sum != 499*500/2 {
		t.Errorf("list has %d nodes with sum %d, want 500 and %d", n, sum, 499*500/2)
	}
}
