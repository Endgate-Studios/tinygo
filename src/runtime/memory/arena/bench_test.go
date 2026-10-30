package arena

import (
	"runtime"
	"testing"
	"unsafe"

	"runtime/memory"
)

var benchSink unsafe.Pointer

// BenchmarkAllocFree compares one 64-byte object from each strategy. The GC case
// includes its share of the collections.
func BenchmarkAllocFree(b *testing.B) {
	b.Run("arena.Heap", func(b *testing.B) {
		h, _ := NewHeap(make([]byte, 64<<10))
		for range b.N {
			p := h.Alloc(64, 0)
			h.Free(p)
		}
	})
	b.Run("arena.Region", func(b *testing.B) {
		r, _ := NewRegion(make([]byte, 64<<10))
		for range b.N {
			if benchSink = r.Alloc(64, 0); benchSink == nil {
				r.Reset()
			}
		}
	})
	b.Run("gc", func(b *testing.B) {
		for i := range b.N {
			p := new([64]byte)
			p[i%64] = byte(i)
			benchSink = unsafe.Pointer(p)
		}
	})
	benchSink = nil
}

// bnode is a list node with a pointer, so that the precise GC must read it.
type bnode struct {
	next *bnode
	data [7]uintptr
}

var benchList *bnode

// liveNodes is the number of list nodes in 256 KiB.
const liveNodes = (256 << 10) / unsafe.Sizeof(bnode{})

// BenchmarkGCWithLiveData measures one collection while 256 KiB of list nodes is live in
// the GC heap or in an arena. bench_static_test.go adds an arena outside the GC heap.
func BenchmarkGCWithLiveData(b *testing.B) {
	const n = liveNodes
	b.Run("gc heap", func(b *testing.B) {
		for range n {
			benchList = &bnode{next: benchList}
		}
		runtime.GC()
		b.ResetTimer()
		for range b.N {
			runtime.GC()
		}
		b.StopTimer()
		benchList = nil
	})
	b.Run("arena over make", func(b *testing.B) {
		benchArenaList(b, make([]byte, arenaBytes))
	})
}

// arenaBytes holds liveNodes with their boundary tags.
const arenaBytes = liveNodes * (unsafe.Sizeof(bnode{}) + 4*wordSize)

func benchArenaList(b *testing.B, mem []byte) {
	h, err := NewHeap(mem)
	if err != nil {
		b.Fatal(err)
	}
	var head *bnode
	for range liveNodes {
		p := memory.New[bnode](h)
		if p == nil {
			b.Fatal("arena full")
		}
		p.next, head = head, p
	}
	runtime.GC()
	b.ResetTimer()
	for range b.N {
		runtime.GC()
	}
	b.StopTimer()
	if head == nil {
		b.Fatal("list lost")
	}
}

// BenchmarkChurn allocates and drops objects of 16 to 700 bytes with 64 live at a time.
// peak-B is the highest HeapInuse of the GC heap.
func BenchmarkChurn(b *testing.B) {
	var live [64]unsafe.Pointer
	b.Run("gc", func(b *testing.B) {
		peak := uint64(0)
		for i := range b.N {
			live[i%len(live)] = unsafe.Pointer(unsafe.SliceData(make([]byte, 16+i%685)))
			if i%1024 == 0 {
				peak = max(peak, heapInuse())
			}
		}
		b.ReportMetric(float64(peak), "peak-B")
		clear(live[:])
	})
	b.Run("arena.Heap", func(b *testing.B) {
		h, _ := NewHeap(make([]byte, 128<<10))
		peak := uint64(0)
		for i := range b.N {
			slot := &live[i%len(live)]
			if *slot != nil {
				h.Free(*slot)
			}
			*slot = h.Alloc(uintptr(16+i%685), 0)
			if i%1024 == 0 {
				peak = max(peak, heapInuse())
			}
		}
		b.ReportMetric(float64(peak), "peak-B")
		b.ReportMetric(float64(h.Stats().Peak), "arena-peak-B")
		clear(live[:])
	})
}

func heapInuse() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}
