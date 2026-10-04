// Command gcsuite runs the collector correctness tests on a real TinyGo runtime with any -gc setting.
// The quick argument divides loop counts by 100 for gcshim_stress builds. README.md has the commands.
package main

import (
	"os"
	"runtime"
	"time"
	"unsafe"

	"runtime/memory"
	"runtime/memory/arena"
	"runtime/memory/free"
	"runtime/memory/gcshim"
	"runtime/memory/heap"
)

type node struct {
	next *node
	val  int
	pad  [3]int
}

var (
	failures   int
	globalRoot *node
	sink       []byte

	// scale divides the long loop counts. The quick argument sets it to 100.
	scale = 1

	// batch is the number of goroutines alive at the same time.
	batch = 50
)

func check(name string, ok bool) {
	if ok {
		println("PASS", name)
		return
	}
	println("FAIL", name)
	failures++
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "quick" {
		scale = 100
	}
	println("gcshim installed:", gcshim.Installed(), "real free:", free.IsReal())

	testListSurvivesCollection()
	testInteriorPointer()
	testGlobalRoot()
	testGoroutineStackRoots()
	testManyGoroutines()
	testSizes()
	testSteadyState()
	testFreeShim()
	testArenaOnReservedMemory()
	testInjectedStrategies()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	println("NumGC", ms.NumGC, "Mallocs", ms.Mallocs, "Frees", ms.Frees, "HeapSys", ms.HeapSys, "HeapInuse", ms.HeapInuse)
	if gcshim.Installed() {
		s := gcshim.ReadStats()
		println("pause last", s.LastPauseNs, "max", s.MaxPauseNs, "total", s.TotalPauseNs, "ns")
	}
	if failures > 0 {
		println(failures, "FAILED")
		os.Exit(1)
	}
	println("ALL PASS")
}

//go:noinline
func buildList(n, seed int) *node {
	var head *node
	for i := 0; i < n; i++ {
		head = &node{next: head, val: i*seed + 1}
	}
	return head
}

func sumList(l *node) (sum, count int) {
	for ; l != nil; l = l.next {
		sum += l.val
		count++
	}
	return
}

func expectedSum(n, seed int) int {
	s := 0
	for i := 0; i < n; i++ {
		s += i*seed + 1
	}
	return s
}

func testListSurvivesCollection() {
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	list := buildList(5000, 3)
	// Drop the second half and unlink each dropped node. With a conservative scan,
	// one stale word to a dropped node would keep the rest of the chain alive.
	cut := list
	for i := 0; i < 2499; i++ {
		cut = cut.next
	}
	for dropped := cut.next; dropped != nil; {
		next := dropped.next
		dropped.next = nil
		dropped = next
	}
	cut.next = nil
	runtime.GC()
	churn()
	runtime.GC()

	sum, count := sumList(list)
	check("list survives GC with intact data", count == 2500 && sum == sumTop(5000, 3, 2500))

	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	check("dropped half is freed", after.Frees-before.Frees >= 2000)
}

// sumTop returns the sum of the newest k values of a list from buildList.
func sumTop(n, seed, k int) int {
	s := 0
	for i := n - k; i < n; i++ {
		s += i*seed + 1
	}
	return s
}

// churn allocates garbage so that freed slots are used again and a dangling
// pointer shows as bad data.
//
//go:noinline
func churn() {
	for i := 0; i < 20000/scale; i++ {
		sink = make([]byte, 24+i%200)
		for j := range sink {
			sink[j] = 0xEE
		}
	}
	sink = nil
}

func testInteriorPointer() {
	buf := make([]int, 1000)
	for i := range buf {
		buf[i] = i
	}
	mid := &buf[500]
	buf = nil
	runtime.GC()
	churn()
	runtime.GC()
	back := unsafe.Slice((*int)(unsafe.Add(unsafe.Pointer(mid), -500*int(unsafe.Sizeof(0)))), 1000)
	ok := true
	for i, v := range back {
		if v != i {
			ok = false
			break
		}
	}
	check("interior pointer keeps whole object alive", ok)
}

func testGlobalRoot() {
	globalRoot = buildList(1000, 7)
	runtime.GC()
	churn()
	runtime.GC()
	sum, count := sumList(globalRoot)
	check("object reachable only from a global survives", count == 1000 && sum == expectedSum(1000, 7))
	globalRoot = nil
}

func testGoroutineStackRoots() {
	done := make(chan bool)
	for g := 0; g < 8; g++ {
		go func(seed int) {
			l := buildList(300, seed)
			time.Sleep(5 * time.Millisecond) // main collects during the sleep
			sum, count := sumList(l)
			done <- count == 300 && sum == expectedSum(300, seed)
		}(g + 2)
	}
	for i := 0; i < 5; i++ {
		runtime.GC()
		churn()
		time.Sleep(time.Millisecond)
	}
	ok := true
	for g := 0; g < 8; g++ {
		ok = <-done && ok
	}
	check("objects held only on goroutine stacks survive", ok)
}

// testManyGoroutines starts and ends goroutines between collections and checks their values.
// test/gcgoroutines measures stack retention. NOTES.md tells why.
func testManyGoroutines() {
	done := make(chan int)
	ok := true
	for round := 0; round < 20/min(scale, 10); round++ {
		sum := 0
		for i := 0; i < batch; i++ {
			go func(v int) { done <- v }(i)
		}
		for i := 0; i < batch; i++ {
			sum += <-done
		}
		ok = ok && sum == batch*(batch-1)/2
		runtime.GC()
	}
	check("goroutines start, run and exit across collections", ok)
}

func testSizes() {
	ok := true
	zero := make([]byte, 0)
	ok = ok && zero != nil
	for _, n := range []int{1, 15, 16, 17, 48, 512, 1024, 2048, 2049, 4096, 100000} {
		b := make([]byte, n)
		for i := range b {
			if b[i] != 0 {
				ok = false
			}
			b[i] = byte(i)
		}
		sink = b
		runtime.GC()
		for i := range sink {
			if sink[i] != byte(i) {
				ok = false
			}
		}
	}
	sink = nil
	check("zero, class-boundary and large sizes are zeroed and kept", ok)
}

func testSteadyState() {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 300000/scale; i++ {
		sink = make([]byte, 16+i%700)
	}
	sink = nil
	runtime.GC()
	runtime.ReadMemStats(&after)
	println("  steady state: HeapInuse", before.HeapInuse, "->", after.HeapInuse, "HeapSys", before.HeapSys, "->", after.HeapSys)
	check("steady-state loop keeps heap bounded", after.HeapInuse < before.HeapInuse+128<<10)
}

func testFreeShim() {
	if !free.IsReal() {
		check("free shim is a safe no-op without gcshim", !free.Verify())
		return
	}
	check("free shim Verify", free.Verify())

	sink = make([]byte, 4096)
	first := unsafe.SliceData(sink)
	free.FreeSlice(sink)
	sink = make([]byte, 4096)
	check("freed memory is reused immediately", unsafe.SliceData(sink) == first)
	sink = nil

	n := new(node)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	free.Free(n)
	runtime.ReadMemStats(&after)
	check("free.Free counts in MemStats.Frees", after.Frees == before.Frees+1)
}

func testArenaOnReservedMemory() {
	mem := gcshim.Reserve(32 << 10)
	h, err := arena.NewHeap(mem)
	if err != nil {
		check("arena on reserved memory", false)
		return
	}
	type rec struct{ a, b, c int }
	recs := memory.MakeSlice[rec](h, 500)
	for i := range recs {
		recs[i] = rec{i, i * 2, i * 3}
	}
	runtime.GC()
	churn()
	runtime.GC()
	ok := h.Check() == nil
	for i, r := range recs {
		if r != (rec{i, i * 2, i * 3}) {
			ok = false
			break
		}
	}
	memory.FreeSlice(h, recs)
	check("arena on gcshim.Reserve memory survives collections", ok && h.Stats().InUse == 0)
	gcshim.Release(mem)
}

// buffers is a consumer interface. It declares only what it needs, so any strategy fits.
type buffers interface {
	memory.Allocator
	memory.Freer
}

func fillAndRelease(mem buffers, n int) bool {
	b := memory.MakeSlice[uint32](mem, n)
	if b == nil {
		return false
	}
	for i := range b {
		b[i] = uint32(i)
	}
	sum := uint32(0)
	for _, v := range b {
		sum += v
	}
	memory.FreeSlice(mem, b)
	return sum == uint32(n*(n-1)/2)
}

func testInjectedStrategies() {
	gc, err := heap.New()
	if err != nil {
		check("heap.New", false)
		return
	}
	h, err := arena.NewHeap(make([]byte, 16<<10))
	if err != nil {
		check("arena.NewHeap", false)
		return
	}
	check("consumer runs on heap strategy", fillAndRelease(gc, 1000))
	check("consumer runs on arena strategy", fillAndRelease(h, 1000))
}
