package memtest_test

import (
	"testing"
	"unsafe"

	"runtime/memory"
	"runtime/memory/arena"
	"runtime/memory/free"
	"runtime/memory/gcshim"
	"runtime/memory/heap"
	"runtime/memory/internal/memtest"
)

// strategy is one way to get memory and give it back.
type strategy struct {
	name string
	skip string // why this build cannot run the strategy, or ""
	// open returns an allocator over about n bytes, and a release that gives back all it gave.
	open func(t testing.TB, n uintptr) (memory.Allocator, func())
}

func strategies() []strategy {
	return []strategy{
		{name: "gc", open: openGC},
		{name: "free", skip: unless(free.IsReal(), "this build has no real free"), open: openFree},
		{name: "arena.Heap", open: openArenaHeap},
		{name: "arena.Region", open: openRegion},
		{name: "region-root", skip: unless(gcshim.Installed(), "gcshim is not the collector"), open: openRegionRoot},
		{name: "scope", skip: unless(gcshim.Installed(), "gcshim is not the collector"), open: openScope},
	}
}

// forEach runs f as a subtest for each strategy that this build can run.
func forEach(t *testing.T, f func(t *testing.T, s strategy)) {
	for _, s := range strategies() {
		t.Run(s.name, func(t *testing.T) {
			if s.skip != "" {
				t.Skip(s.skip)
			}
			f(t, s)
		})
	}
}

func unless(ok bool, why string) string {
	if ok {
		return ""
	}
	return why
}

func openGC(t testing.TB, n uintptr) (memory.Allocator, func()) {
	h, err := heap.New()
	if err != nil {
		t.Fatal(err)
	}
	return h, func() {}
}

func openFree(t testing.TB, n uintptr) (memory.Allocator, func()) {
	h, err := heap.New()
	if err != nil {
		t.Fatal(err)
	}
	r := &recorder{mem: h}
	return r, r.release
}

func openArenaHeap(t testing.TB, n uintptr) (memory.Allocator, func()) {
	h, err := arena.NewHeap(make([]byte, n))
	if err != nil {
		t.Fatal(err)
	}
	r := &recorder{mem: h}
	return r, r.release
}

func openRegion(t testing.TB, n uintptr) (memory.Allocator, func()) {
	r, err := arena.NewRegion(make([]byte, n))
	if err != nil {
		t.Fatal(err)
	}
	return r, r.Reset
}

func openRegionRoot(t testing.TB, n uintptr) (memory.Allocator, func()) {
	r, err := arena.NewRegion(make([]byte, n))
	if err != nil {
		t.Fatal(err)
	}
	if err := gcshim.AddRoot(r); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gcshim.RemoveRoot(r) })
	return r, r.Reset
}

// scopeMem is not heap memory, as NewScope requires.
var scopeMem [64 << 10 / 8]uint64

func openScope(t testing.TB, n uintptr) (memory.Allocator, func()) {
	mem := unsafe.Slice((*byte)(unsafe.Pointer(&scopeMem[0])), min(n, unsafe.Sizeof(scopeMem)))
	s := gcshim.NewScope(mem)
	if s == nil {
		t.Fatal("NewScope = nil: MaxRoots roots are registered")
	}
	t.Cleanup(func() { gcshim.RemoveRoot(s) })
	return scopeAllocator{s}, s.Reset
}

// scopeAllocator allocates in a Scope through the GC heap, as code inside the Scope does.
type scopeAllocator struct{ s *gcshim.Scope }

func (a scopeAllocator) Alloc(size, align uintptr) unsafe.Pointer {
	if align > 8 || align&(align-1) != 0 {
		return nil
	}
	a.s.Enter()
	b := make([]byte, size)
	a.s.Exit()
	return unsafe.Pointer(unsafe.SliceData(b))
}

// recorder keeps each allocation so that release can free all of them.
type recorder struct {
	mem interface {
		memory.Allocator
		memory.Freer
	}
	live []unsafe.Pointer
}

func (r *recorder) Alloc(size, align uintptr) unsafe.Pointer {
	p := r.mem.Alloc(size, align)
	if p != nil {
		r.live = append(r.live, p)
	}
	return p
}

func (r *recorder) release() {
	for _, p := range r.live {
		r.mem.Free(p)
	}
	clear(r.live)
	r.live = r.live[:0]
}

// TestStrategiesGiveZeroedAlignedMemory checks the Allocator contract of each strategy,
// also for memory that a release gave back and the next allocation uses again.
func TestStrategiesGiveZeroedAlignedMemory(t *testing.T) {
	forEach(t, func(t *testing.T, s strategy) {
		a, release := s.open(t, 32<<10)
		for round := range 3 {
			for i := range 100 {
				size := uintptr(1 + i*7%200)
				align := uintptr(1) << (i % 4)
				p := a.Alloc(size, align)
				if p == nil {
					t.Fatalf("round %d: Alloc(%d, %d) = nil", round, size, align)
				}
				if uintptr(p)%align != 0 {
					t.Fatalf("round %d: Alloc(%d, %d) = %#x, not aligned", round, size, align, p)
				}
				b := unsafe.Slice((*byte)(p), size)
				for j, v := range b {
					if v != 0 {
						t.Fatalf("round %d: Alloc(%d, %d) byte %d = %#x, not zeroed", round, size, align, j, v)
					}
				}
				for j := range b {
					b[j] = 0xFF
				}
			}
			release()
		}
	})
}

// TestHeldVictimsSurvive checks the victim helpers. Victims held in a slice that the GC
// reads must all survive, with every collector.
func TestHeldVictimsSurvive(t *testing.T) {
	slots := make([]unsafe.Pointer, 16)
	memtest.Plant(slots)
	if bad := memtest.Collect(slots, 8); bad != 0 {
		t.Errorf("%d of %d held victims were overwritten", bad, len(slots))
	}
}
