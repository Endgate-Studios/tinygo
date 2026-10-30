package main

import (
	"runtime"
	"unsafe"

	"runtime/memory"
	"runtime/memory/arena"
	"runtime/memory/free"
	"runtime/memory/gcshim"
	"runtime/memory/heap"
)

// victim is a GC heap object with a known pattern. gcshim has no finalizers, so the
// tests find a freed victim when a new object changes its pattern.
type victim struct {
	canary [8]uintptr
}

const (
	canaryWord = 0xA5A5A5A5
	victims    = 16
)

var (
	staticArena [8 << 10]byte
	victimSink  *victim
)

//go:noinline
func plantVictims(slots []unsafe.Pointer) {
	for i := range slots {
		v := new(victim)
		for j := range v.canary {
			v.canary[j] = canaryWord
		}
		slots[i] = unsafe.Pointer(v)
	}
}

// clobberStack writes over old copies of the victim pointers on the stack.
//
//go:noinline
func clobberStack() uintptr {
	var buf [256]uintptr
	for i := range buf {
		buf[i] = uintptr(i)
	}
	return buf[len(buf)-1]
}

// reuseFreed allocates objects of the victim size with a different pattern.
//
//go:noinline
func reuseFreed() {
	for range 2000 {
		v := new(victim)
		for j := range v.canary {
			v.canary[j] = 0x5A5A5A5A
		}
		victimSink = v
	}
	victimSink = nil
}

func corruptedVictims(slots []unsafe.Pointer) int {
	n := 0
	for _, p := range slots {
		for _, w := range (*victim)(p).canary {
			if w != canaryWord {
				n++
				break
			}
		}
	}
	return n
}

// arenaCase is arena memory and whether the GC reads it.
type arenaCase struct {
	name string
	mem  []byte
	read bool
}

// testGCPointerOnlyInArena puts the only pointer to each victim in arena memory. A victim
// survives only where the GC reads that memory.
func testGCPointerOnlyInArena() {
	if !gcKnown {
		println("SKIP GC pointer in arena: unknown collector")
		return
	}
	reserved := gcshim.Reserve(4 << 10)
	cases := []arenaCase{
		{"make", make([]byte, 4<<10), heapBytesRead},
		{"gcshim.Reserve", reserved, heapBytesRead && !gcshim.Installed()},
		{"static array", staticArena[:], globalBytesRead},
	}
	if mem := psramArena(); mem != nil {
		cases = append(cases, arenaCase{"PSRAM", mem, false})
	}
	for _, c := range cases {
		h, err := arena.NewHeap(c.mem)
		if err != nil {
			check("arena.NewHeap over "+c.name, false)
			continue
		}
		slots := memory.MakeSlice[unsafe.Pointer](h, victims)
		plantVictims(slots)
		clobberStack()
		// The allocator can use freed blocks again only after some rounds.
		bad := 0
		for round := 0; round < 8 && bad == 0; round++ {
			runtime.GC()
			reuseFreed()
			bad = corruptedVictims(slots)
		}
		println("  arena over", c.name+":", bad, "of", victims, "victims overwritten")
		if c.read {
			check("GC reads the "+c.name+" arena, so its victims survive", bad == 0)
		} else {
			check("GC does not read the "+c.name+" arena, so its victims are freed", bad > 0)
		}
	}
	gcshim.Release(reserved)
}

// testHeapStrategyEarlyFree frees objects early through the heap strategy, then checks
// that later collections keep the live data.
func testHeapStrategyEarlyFree() {
	h, err := heap.New()
	if err != nil {
		check("heap.New", false)
		return
	}
	live := buildList(500, 3)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := range 1000 / min(scale, 10) {
		b := memory.MakeSlice[byte](h, 100)
		b[0] = byte(i)
		memory.FreeSlice(h, b)
	}
	runtime.ReadMemStats(&after)
	if free.IsReal() {
		check("heap strategy Free counts in MemStats.Frees", after.Frees-before.Frees >= uint64(1000/min(scale, 10)))
	}
	for range 3 {
		runtime.GC()
		churn()
	}
	sum, count := sumList(live)
	check("early frees through the heap strategy keep live data across collections", count == 500 && sum == expectedSum(500, 3))
}
