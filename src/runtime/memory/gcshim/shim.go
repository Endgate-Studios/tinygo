//go:build gc.custom && (tinygo.wasm || (baremetal && (esp32 || esp32s3 || esp32c3 || scheduler.cores)) || (windows && amd64 && scheduler.tasks))

package gcshim

import (
	"runtime"
	"unsafe"

	"runtime/memory/collector"
)

// shim holds all mutable gcshim state. markRoots skips it, because it has no GC pointers
// and its counters look like heap addresses (NOTES.md, Bugs found while testing).
var shim struct {
	spans      heapSpans // collectors of a multi-span heap, empty in other builds
	heap       gcHeap
	pauses     pauseStats
	allocCount uintptr
	allocated  uintptr  // bytes allocated since the last collection
	lock       heapLock // locks the heap across cores, does nothing on one core
}

// spillAfter is how much must be allocated since the last collection before
// a full first span collects rather than spill into the next (PSRAM, which
// is slower and which DMA can't reach). Without it, a first span full of
// live data would collect at every allocation. Each collection stops every
// core for the mark, which takes as long as the live data does, not the
// garbage: so rarely is better, while PSRAM has room. At 64 KB a UI that
// rebuilt a few times a second collected more than once a second, and its
// animations stuttered.
const spillAfter = 1 << 20

type pauseStats struct {
	last, max, total uint64
}

//go:linkname runtimePanic runtime.runtimePanic
func runtimePanic(msg string)

//go:linkname nanotime runtime.nanotime
func nanotime() int64

//go:linkname initHeap runtime.initHeap
func initHeap() {
	if err := initArena(); err != nil {
		runtimePanic("gcshim: heap init failed")
	}
}

// pointerFree says a layout is of memory with no pointers, as in runtime/gc_precise.go.
func pointerFree(layout unsafe.Pointer) bool {
	const sizeFieldBits = 4 + unsafe.Sizeof(uintptr(0))/4
	return uintptr(layout)&1 != 0 && uintptr(layout)>>(sizeFieldBits+1) == 0
}

// allocAny allocates from any span.
func allocAny(size uintptr, noPointers bool) unsafe.Pointer {
	if noPointers {
		return shim.heap.AllocBytes(size)
	}
	return shim.heap.Alloc(size)
}

//go:linkname alloc runtime.alloc
func alloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer {
	checkAllocContext()
	noPointers := pointerFree(layout)
	shim.lock.Lock()
	if stressEvery != 0 {
		shim.allocCount++
		if shim.allocCount >= stressEvery {
			shim.allocCount = 0
			collect()
		}
	}
	if p := scopeAlloc(size); p != nil {
		countAlloc(size)
		shim.lock.Unlock()
		return p
	}
	p := allocFirst(size, noPointers)
	if p == nil && shim.allocated >= spillAfter {
		// The first span is full, mostly of garbage by now: collect before
		// spilling into the next one, or it fills with garbage too.
		collect()
		p = allocFirst(size, noPointers)
	}
	if p == nil {
		p = allocAny(size, noPointers)
	}
	if p == nil {
		collect()
		for p = allocAny(size, noPointers); p == nil; p = allocAny(size, noPointers) {
			if !growArena(size) {
				shim.lock.Unlock()
				runtimePanic("out of memory")
			}
		}
	}
	shim.allocated += size
	countAlloc(size)
	shim.lock.Unlock()
	return p
}

//go:linkname free runtime.free
func free(ptr unsafe.Pointer) {
	if ptr == nil {
		return
	}
	shim.lock.Lock()
	ok := shim.heap.Free(ptr)
	inHeap := shim.heap.Contains(ptr)
	shim.lock.Unlock()
	if !ok && strictFree && inHeap {
		runtimePanic("free: invalid or double free")
	}
}

//go:linkname markRoots runtime.markRoots
func markRoots(start, end uintptr) {
	// The runtime gives stacks and globals here. Only the globals can contain shim, so skip it.
	lo := uintptr(unsafe.Pointer(&shim))
	hi := lo + unsafe.Sizeof(shim)
	if lo >= start && hi <= end {
		shim.heap.MarkRange(start, lo)
		shim.heap.MarkRange(hi, end)
		return
	}
	shim.heap.MarkRange(start, end)
}

//go:linkname GC runtime.GC
func GC() {
	shim.lock.Lock()
	collect()
	shim.lock.Unlock()
}

// collect runs one cycle while the caller holds shim.lock. Other cores resume after the mark,
// because the sweep changes only heap metadata and they cannot allocate without the lock.
func collect() {
	t0 := nanotime()
	shim.allocated = 0
	shim.heap.BeginCycle()
	markAllRoots()
	markRegisteredRoots()
	shim.heap.FinishMark()
	resumeWorld()
	shim.heap.Sweep()
	recordPause(uint64(nanotime() - t0))

	// Grow the heap if live data is more than half of it after a collection, as Go does with
	// GOGC=100 (https://go.dev/doc/gc-guide#GOGC). If not, the next collection comes too soon.
	if s := shim.heap.Stats(); s.HeapInuse*2 > s.HeapSys {
		growArena(0)
	}
}

//go:linkname SetFinalizer runtime.SetFinalizer
func SetFinalizer(obj interface{}, finalizer interface{}) {
	// Finalizers are not supported. The contract in src/runtime/gc_custom.go permits a no-op.
}

//go:linkname ReadMemStats runtime.ReadMemStats
func ReadMemStats(ms *runtime.MemStats) {
	shim.lock.Lock()
	s := shim.heap.Stats()
	shim.lock.Unlock()
	// Clear all fields so that fields gcshim does not know read 0 (tinygo-org/tinygo#5785).
	clear(unsafe.Slice((*byte)(unsafe.Pointer(ms)), unsafe.Sizeof(*ms)))
	ms.HeapSys = s.HeapSys
	ms.HeapAlloc = s.HeapInuse
	ms.HeapInuse = s.HeapInuse + s.HeapReserved
	ms.HeapIdle = s.HeapIdle
	ms.HeapReleased = 0
	ms.HeapObjects = s.Mallocs - s.Frees
	ms.Alloc = s.HeapInuse
	ms.TotalAlloc = s.TotalAlloc
	ms.Mallocs = s.Mallocs
	ms.Frees = s.Frees
	ms.NumGC = s.NumGC
	ms.GCSys = s.MetaSys
	ms.Sys = s.HeapSys + s.MetaSys
}

func recordPause(ns uint64) {
	shim.pauses.last = ns
	shim.pauses.total += ns
	if ns > shim.pauses.max {
		shim.pauses.max = ns
	}
}

func collectorConfig() collector.Config {
	return collector.Config{PageSize: pageSize, WorklistLen: worklistLen, Poison: poison}
}

// Installed reports whether gcshim is this program's garbage collector.
func Installed() bool { return true }

// Reserve takes n bytes of unscanned pages from the GC heap, or returns nil if no run is free.
// Do not keep the only reference to a GC heap object in this memory.
func Reserve(n uintptr) []byte {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	mem := shim.heap.Reserve(n)
	if mem == nil {
		collect()
		mem = shim.heap.Reserve(n)
	}
	return mem
}

// Release gives back memory from Reserve.
func Release(mem []byte) {
	if len(mem) != 0 {
		shim.lock.Lock()
		shim.heap.Release(unsafe.Pointer(unsafe.SliceData(mem)))
		shim.lock.Unlock()
	}
}

// ReadStats returns the collector counters and the pause times.
func ReadStats() Stats {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return Stats{
		Stats:        shim.heap.Stats(),
		LastPauseNs:  shim.pauses.last,
		MaxPauseNs:   shim.pauses.max,
		TotalPauseNs: shim.pauses.total,
	}
}
