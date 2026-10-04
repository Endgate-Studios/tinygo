package collector

import (
	"errors"
	"runtime"
	"testing"
	"unsafe"
)

// testSet is a set over two host spans, with roots that the test controls.
type testSet struct {
	t     *testing.T
	a, b  *testHeap
	s     Set
	roots []uintptr
}

func newTestSet(t *testing.T, bytesA, bytesB, worklist uintptr) *testSet {
	t.Helper()
	ts := &testSet{t: t, a: newTestHeap(t, bytesA, 1024, worklist), b: newTestHeap(t, bytesB, 4096, worklist), roots: make([]uintptr, 64)}
	s, err := NewSet(&ts.a.c, &ts.b.c)
	if err != nil {
		t.Fatalf("NewSet: %v", err)
	}
	ts.s = s
	return ts
}

func (ts *testSet) collect() {
	ts.t.Helper()
	lo := uintptr(unsafe.Pointer(&ts.roots[0]))
	ts.s.BeginCycle()
	ts.s.MarkRange(lo, lo+uintptr(len(ts.roots))*wordSize)
	ts.s.FinishMark()
	ts.s.Sweep()
	if err := ts.s.Check(); err != nil {
		ts.t.Fatalf("Check: %v", err)
	}
}

// allocIn allocates from span a or span b directly, so that the test controls the placement.
func (ts *testSet) allocIn(h *testHeap, size uintptr) uintptr {
	ts.t.Helper()
	return h.alloc(size)
}

func (ts *testSet) live(p uintptr) bool {
	if ts.b.c.Contains(unsafe.Pointer(p)) {
		return ts.b.allocated(p)
	}
	return ts.a.allocated(p)
}

func TestNewSetValidates(t *testing.T) {
	a := newTestHeap(t, 64<<10, 1024, 16)
	b := newTestHeap(t, 64<<10, 1024, 16)
	var empty Collector
	tests := []struct {
		name      string
		primary   *Collector
		secondary *Collector
		wantErr   error
	}{
		{name: "nil primary", primary: nil, secondary: &b.c, wantErr: ErrBadSet},
		{name: "uninitialised primary", primary: &empty, wantErr: ErrBadSet},
		{name: "same collector", primary: &a.c, secondary: &a.c, wantErr: ErrBadSet},
		{name: "uninitialised secondary", primary: &a.c, secondary: &empty, wantErr: ErrBadSet},
		{name: "primary only", primary: &a.c},
		{name: "two spans", primary: &a.c, secondary: &b.c},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSet(tt.primary, tt.secondary); !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewSet() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewSetRejectsOverlap(t *testing.T) {
	span := testSpan(t, 256<<10)
	start := uintptr(unsafe.Pointer(&span[0]))
	var a, b Collector
	if err := a.Init(start, start+128<<10, start+192<<10, Config{PageSize: 1024}); err != nil {
		t.Fatal(err)
	}
	// b starts inside the growth limit of a.
	if err := b.Init(start+160<<10, start+256<<10, start+256<<10, Config{PageSize: 1024}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSet(&a, &b); !errors.Is(err, ErrBadSet) {
		t.Fatalf("NewSet() error = %v, want %v", err, ErrBadSet)
	}
	runtime.KeepAlive(span)
}

func TestSetAllocSpillsToSecondary(t *testing.T) {
	ts := newTestSet(t, 64<<10, 128<<10, 16)
	var inA, inB int
	for i := 0; i < 2000; i++ {
		p := ts.s.Alloc(48)
		if p == nil {
			t.Fatalf("Alloc %d = nil", i)
		}
		if ts.b.c.Contains(p) {
			inB++
			continue
		}
		if inB != 0 {
			t.Fatal("Alloc used the primary span after it spilled to the secondary span")
		}
		inA++
	}
	if inA == 0 || inB == 0 {
		t.Fatalf("allocations in primary = %d, secondary = %d, want both > 0", inA, inB)
	}
}

func TestSetCrossSpanReferences(t *testing.T) {
	tests := []struct {
		name     string
		worklist uintptr
		width    uintptr // pointers in each wide object, to force a worklist overflow
	}{
		{name: "large worklist", worklist: 256, width: 4},
		{name: "overflow across spans", worklist: 2, width: 96},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestSet(t, 256<<10, 512<<10, tt.worklist)

			// root (a) -> wide (b) -> mids (a) -> leaves (b) -> tails (a).
			root := ts.allocIn(ts.a, 16)
			wide := ts.allocIn(ts.b, tt.width*wordSize)
			*word(root, 0) = wide
			var chain []uintptr
			for i := uintptr(0); i < tt.width; i++ {
				mid := ts.allocIn(ts.a, 32)
				leaf := ts.allocIn(ts.b, 32)
				tail := ts.allocIn(ts.a, 16)
				*word(wide, i) = mid
				*word(mid, 0) = leaf
				*word(leaf, 0) = tail
				*word(tail, 1) = i + 1000
				chain = append(chain, mid, leaf, tail)
			}
			garbageA := ts.allocIn(ts.a, 3000)
			garbageB := ts.allocIn(ts.b, 64)
			*word(garbageB, 0) = garbageA // garbage that refers across spans stays garbage

			ts.roots[0] = root
			ts.collect()

			for i, p := range append([]uintptr{root, wide}, chain...) {
				if !ts.live(p) {
					t.Fatalf("object %d at %#x freed while reachable", i, p)
				}
			}
			for i := uintptr(0); i < tt.width; i++ {
				if got := *word(chain[3*i+2], 1); got != i+1000 {
					t.Fatalf("tail %d value = %d, want %d", i, got, i+1000)
				}
			}
			if ts.live(garbageA) || ts.live(garbageB) {
				t.Errorf("garbage survived: primary = %v, secondary = %v", ts.live(garbageA), ts.live(garbageB))
			}
		})
	}
}

func TestSetFreeRoutesByAddress(t *testing.T) {
	ts := newTestSet(t, 64<<10, 64<<10, 16)
	pa := ts.allocIn(ts.a, 64)
	pb := ts.allocIn(ts.b, 64)
	var local int
	tests := []struct {
		name string
		p    uintptr
		want bool
		h    *testHeap
	}{
		{name: "primary object", p: pa, want: true, h: ts.a},
		{name: "secondary object", p: pb, want: true, h: ts.b},
		{name: "secondary double free", p: pb, want: false, h: ts.b},
		{name: "interior of secondary", p: ts.allocIn(ts.b, 64) + 8, want: false, h: ts.b},
		{name: "outside both spans", p: uintptr(unsafe.Pointer(&local)), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var before uint64
			if tt.h != nil {
				before = tt.h.c.Stats().Frees
			}
			if got := ts.s.Free(unsafe.Pointer(tt.p)); got != tt.want {
				t.Fatalf("Free(%#x) = %v, want %v", tt.p, got, tt.want)
			}
			if tt.want && tt.h.c.Stats().Frees != before+1 {
				t.Error("Free did not reach the span that holds the object")
			}
		})
	}
	if ts.s.Contains(unsafe.Pointer(&local)) {
		t.Error("Contains(local) = true")
	}
	if !ts.s.Contains(unsafe.Pointer(pb)) || !ts.s.Contains(unsafe.Pointer(pa)) {
		t.Error("Contains is false for a span address")
	}
}

func TestSetStatsSum(t *testing.T) {
	ts := newTestSet(t, 64<<10, 128<<10, 16)
	ts.allocIn(ts.a, 100)
	ts.allocIn(ts.b, 5000)
	if ts.s.Reserve(2000) == nil {
		t.Fatal("Reserve = nil")
	}
	ts.collect()
	ts.collect()

	a, b, got := ts.a.c.Stats(), ts.b.c.Stats(), ts.s.Stats()
	tests := []struct {
		name      string
		got, want uint64
	}{
		{name: "HeapSys", got: got.HeapSys, want: a.HeapSys + b.HeapSys},
		{name: "HeapIdle", got: got.HeapIdle, want: a.HeapIdle + b.HeapIdle},
		{name: "HeapInuse", got: got.HeapInuse, want: a.HeapInuse + b.HeapInuse},
		{name: "HeapReserved", got: got.HeapReserved, want: 2048},
		{name: "MetaSys", got: got.MetaSys, want: a.MetaSys + b.MetaSys},
		{name: "TotalAlloc", got: got.TotalAlloc, want: 5100},
		{name: "Mallocs", got: got.Mallocs, want: 2},
		{name: "Frees", got: got.Frees, want: 2},
		{name: "NumGC", got: uint64(got.NumGC), want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
	if b.HeapReserved != 0 {
		t.Errorf("secondary HeapReserved = %d, want 0 (Reserve is primary only)", b.HeapReserved)
	}
}

func TestSetWithoutSecondaryMatchesCollector(t *testing.T) {
	h := newTestHeap(t, 64<<10, 1024, 16)
	s, err := NewSet(&h.c, nil)
	if err != nil {
		t.Fatal(err)
	}
	keep := uintptr(s.Alloc(64))
	drop := uintptr(s.Alloc(64))
	h.roots[0] = keep
	lo := uintptr(unsafe.Pointer(&h.roots[0]))
	s.BeginCycle()
	s.MarkRange(lo, lo+uintptr(len(h.roots))*wordSize)
	s.FinishMark()
	s.Sweep()
	if !h.allocated(keep) || h.allocated(drop) {
		t.Fatalf("keep live = %v, drop live = %v", h.allocated(keep), h.allocated(drop))
	}
	for s.Alloc(2000) != nil {
	}
	if s.Stats() != h.c.Stats() {
		t.Error("Stats differ from the primary collector")
	}
}
