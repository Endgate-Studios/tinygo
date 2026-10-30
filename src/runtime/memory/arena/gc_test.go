package arena

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"runtime/memory"
)

// victim is a GC heap object. Its finalizer counts it when the GC frees it.
type victim struct {
	canary [8]uintptr
}

const canaryWord = 0xA5A5A5A5

// staticArena is a global with no pointer slots. The GC does not read it in any mode
// (gcGlobalRoot in src/runtime/gc_globals_blocks.go).
var staticArena [16 << 10]byte

// globalVictim keeps a victim alive through a GC root.
var globalVictim *victim

var churnSink []byte

// churn allocates and drops GC memory, so that freed objects are used again.
//
//go:noinline
func churn() {
	for i := range 2000 {
		churnSink = make([]byte, 16+i%200)
		v := new(victim)
		v.canary[0] = uintptr(i)
	}
	churnSink = nil
}

// clobberStack writes over the stack below the caller, so that old copies of
// victim pointers do not keep the victims alive.
//
//go:noinline
func clobberStack() uintptr {
	var buf [512]uintptr
	for i := range buf {
		buf[i] = uintptr(i)
	}
	return buf[len(buf)-1]
}

// collect runs a few cycles and gives the finalizer goroutine time to run.
func collect() {
	clobberStack()
	for range 4 {
		runtime.GC()
		churn()
		time.Sleep(time.Millisecond)
	}
}

// plantVictims stores the only pointer to each new victim in the slots, which are arena memory.
//
//go:noinline
func plantVictims(slots []unsafe.Pointer, freed *int) {
	for i := range slots {
		v := new(victim)
		for j := range v.canary {
			v.canary[j] = canaryWord
		}
		runtime.SetFinalizer(v, func(*victim) { *freed++ })
		slots[i] = unsafe.Pointer(v)
	}
}

func intactVictims(slots []unsafe.Pointer) int {
	n := 0
	for _, p := range slots {
		if (*victim)(p).canary == [8]uintptr{canaryWord, canaryWord, canaryWord, canaryWord, canaryWord, canaryWord, canaryWord, canaryWord} {
			n++
		}
	}
	return n
}

// TestGCPointerOnlyInArena shows the weakness. The GC sees a pointer only in memory that
// it reads, so a victim whose only pointer is in unread arena memory gets freed.
func TestGCPointerOnlyInArena(t *testing.T) {
	if !gcKnown {
		t.Skip("needs -gc=conservative or -gc=precise")
	}
	const n = 32
	tests := []struct {
		name  string
		alloc func(t *testing.T) memory.Allocator
		read  bool // the GC reads this arena memory
	}{
		{"heap over make", func(t *testing.T) memory.Allocator { return newTestHeap(t, 16<<10) }, gcReadsBytes},
		{"region over make", func(t *testing.T) memory.Allocator { return newTestRegion(t, make([]byte, 16<<10)) }, gcReadsBytes},
		{"heap over static array", func(t *testing.T) memory.Allocator {
			h, err := NewHeap(staticArena[:])
			if err != nil {
				t.Fatal(err)
			}
			return h
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slots := memory.MakeSlice[unsafe.Pointer](tt.alloc(t), n)
			if slots == nil {
				t.Fatal("MakeSlice = nil")
			}
			freed := 0
			plantVictims(slots, &freed)
			collect()
			t.Logf("freed %d of %d victims, %d canaries intact", freed, n, intactVictims(slots))
			if tt.read {
				if freed != 0 {
					t.Errorf("the GC reads this arena, but it freed %d of %d victims", freed, n)
				}
				return
			}
			// Old copies on the stack can keep a few victims alive, so ask for half.
			if freed < n/2 {
				t.Errorf("the GC does not read this arena, but it freed only %d of %d victims", freed, n)
			}
		})
	}
}

// anode is a list node that lives in an arena and points only into the same arena.
type anode struct {
	next *anode
	val  int
}

// TestSafeArenaPatternsSurviveGC shows the strength. The patterns that the radio, TLS
// and AES code use keep their data in every GC mode.
func TestSafeArenaPatternsSurviveGC(t *testing.T) {
	if !gcKnown {
		t.Skip("needs -gc=conservative or -gc=precise")
	}
	t.Run("data without pointers", func(t *testing.T) {
		h := newHeapNoRef(16 << 10)
		type rec struct{ a, b, c int }
		recs := memory.MakeSlice[rec](h, 300)
		for i := range recs {
			recs[i] = rec{i, i * 2, i * 3}
		}
		collect()
		for i, r := range recs {
			if r != (rec{i, i * 2, i * 3}) {
				t.Fatalf("recs[%d] = %v", i, r)
			}
		}
		mustCheck(t, h)
	})
	t.Run("pointers into the same arena", func(t *testing.T) {
		h := newHeapNoRef(16 << 10)
		var head *anode
		for i := range 300 {
			n := memory.New[anode](h)
			n.next, n.val = head, i
			head = n
		}
		collect()
		for i := 299; i >= 0; i, head = i-1, head.next {
			if head == nil || head.val != i {
				t.Fatalf("node %d lost", i)
			}
		}
		mustCheck(t, h)
	})
	t.Run("pointer to an object with a GC root", func(t *testing.T) {
		h := newHeapNoRef(4 << 10)
		slots := memory.MakeSlice[unsafe.Pointer](h, 1)
		freed := 0
		plantVictims(slots, &freed)
		globalVictim = (*victim)(slots[0])
		collect()
		if freed != 0 || intactVictims(slots) != 1 {
			t.Errorf("victim with a global root: freed %d, intact %d", freed, intactVictims(slots))
		}
		globalVictim = nil
	})
	t.Run("region keeps its memory", func(t *testing.T) {
		r := newRegionNoRef(8 << 10)
		b := memory.MakeSlice[uint32](r, 1000)
		for i := range b {
			b[i] = uint32(i) * 7
		}
		collect()
		for i, v := range b {
			if v != uint32(i)*7 {
				t.Fatalf("b[%d] = %d", i, v)
			}
		}
	})
}

// newHeapNoRef makes a Heap over GC memory that only the Heap refers to.
//
//go:noinline
func newHeapNoRef(size int) *Heap {
	h, err := NewHeap(make([]byte, size))
	if err != nil {
		panic(err)
	}
	return h
}

//go:noinline
func newRegionNoRef(size int) *Region {
	r, err := NewRegion(make([]byte, size))
	if err != nil {
		panic(err)
	}
	return r
}

func newTestRegion(t *testing.T, mem []byte, opts ...RegionOption) *Region {
	t.Helper()
	r, err := NewRegion(mem, opts...)
	if err != nil {
		t.Fatalf("NewRegion: %v", err)
	}
	return r
}
