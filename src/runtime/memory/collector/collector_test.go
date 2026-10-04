package collector

import (
	"errors"
	"math/rand"
	"runtime"
	"testing"
	"unsafe"
)

// testHeap is a collector over a host span, with roots that the test controls.
// The span is a []uintptr so that the host GC keeps it alive and aligned.
type testHeap struct {
	t     *testing.T
	span  []uintptr
	roots []uintptr
	c     Collector
}

// maxTestHeap is the largest test heap. A board without PSRAM sets it lower in heaplimit_dram_test.go,
// because there the test heaps come from the board heap.
var maxTestHeap = ^uintptr(0)

func newTestHeap(t *testing.T, bytes, pageSize uintptr, worklist uintptr) *testHeap {
	t.Helper()
	if bytes > maxTestHeap {
		t.Skipf("a test heap of %d bytes does not fit, the limit is %d", bytes, maxTestHeap)
	}
	h := &testHeap{t: t, span: testSpan(t, bytes), roots: make([]uintptr, 64)}
	start := uintptr(unsafe.Pointer(&h.span[0]))
	err := h.c.Init(start, start+bytes, start+bytes, Config{PageSize: pageSize, WorklistLen: worklist, Poison: true})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { runtime.KeepAlive(h.span) })
	return h
}

func (h *testHeap) alloc(size uintptr) uintptr {
	h.t.Helper()
	p := h.c.Alloc(size)
	if p == nil {
		h.t.Fatalf("Alloc(%d) = nil", size)
	}
	return uintptr(p)
}

func (h *testHeap) collect() {
	h.t.Helper()
	h.c.BeginCycle()
	h.c.MarkRange(uintptr(unsafe.Pointer(&h.roots[0])), uintptr(unsafe.Pointer(&h.roots[0]))+uintptr(len(h.roots))*wordSize)
	h.c.FinishMark()
	h.c.Sweep()
	h.check()
}

func (h *testHeap) check() {
	h.t.Helper()
	if err := h.c.Check(); err != nil {
		h.t.Fatalf("Check: %v", err)
	}
}

func (h *testHeap) allocated(obj uintptr) bool {
	return getBit(h.c.allocBits, h.c.granuleOf(obj))
}

func word(addr, i uintptr) *uintptr {
	return (*uintptr)(unsafe.Pointer(addr + i*wordSize))
}

func TestInitRejectsBadConfig(t *testing.T) {
	span := make([]uintptr, 4096)
	start := uintptr(unsafe.Pointer(&span[0]))
	end := start + uintptr(len(span))*wordSize

	tests := []struct {
		name    string
		start   uintptr
		end     uintptr
		cfg     Config
		wantErr error
	}{
		{name: "page size not power of two", start: start, end: end, cfg: Config{PageSize: 1000}, wantErr: ErrBadPageSize},
		{name: "page size too small", start: start, end: end, cfg: Config{PageSize: 128}, wantErr: ErrBadPageSize},
		{name: "span too small", start: start, end: start + 512, cfg: Config{PageSize: 1024}, wantErr: ErrHeapTooSmall},
		{name: "ok", start: start, end: end, cfg: Config{PageSize: 1024}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c Collector
			err := c.Init(tt.start, tt.end, tt.end, tt.cfg)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Init() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
	runtime.KeepAlive(span)
}

// TestStacksAlignedOnMisalignedHeapStart covers a heap start 12 bytes past a
// 16 byte boundary, which the ESP32-S3 had before tinygo-org/tinygo#5789.
func TestStacksAlignedOnMisalignedHeapStart(t *testing.T) {
	span := testSpan(t, 128<<10)
	start := alignUp(uintptr(unsafe.Pointer(&span[0])), 16) + 12
	end := start + 120<<10
	var c Collector
	if err := c.Init(start, end, end, Config{PageSize: 1024, WorklistLen: 16}); err != nil {
		t.Fatal(err)
	}
	for _, size := range []uintptr{24, 100, 2048, 4096, 8192} {
		p := uintptr(c.Alloc(size))
		if p == 0 || p%16 != 0 || (p+size)%16 != 0 && size%16 == 0 {
			t.Errorf("Alloc(%d) = %#x, not 16-byte aligned", size, p)
		}
	}
	runtime.KeepAlive(span)
}

func TestAllocZeroedAlignedAndDistinct(t *testing.T) {
	h := newTestHeap(t, 256<<10, 1024, 64)
	sizes := []uintptr{1, 15, 16, 17, 48, 100, 512, 513, 1024, 3000, 10000}
	seen := map[uintptr]bool{}
	for _, size := range sizes {
		p := h.alloc(size)
		if p%granule != 0 {
			t.Errorf("Alloc(%d) = %#x, not %d-aligned", size, p, granule)
		}
		if seen[p] {
			t.Errorf("Alloc(%d) returned %#x twice", size, p)
		}
		seen[p] = true
		for i := uintptr(0); i < size; i++ {
			if b := *(*byte)(unsafe.Pointer(p + i)); b != 0 {
				t.Fatalf("Alloc(%d) byte %d = %#x, want 0", size, i, b)
			}
		}
		for i := uintptr(0); i < size; i++ {
			*(*byte)(unsafe.Pointer(p + i)) = 0xAB
		}
	}
	h.check()
}

func TestZeroSizeAlloc(t *testing.T) {
	h := newTestHeap(t, 64<<10, 1024, 64)
	p := h.c.Alloc(0)
	if p == nil {
		t.Fatal("Alloc(0) = nil, want non-nil sentinel")
	}
	if h.c.Free(p) {
		t.Error("Free(zero-size sentinel) = true, want false")
	}
}

func TestExplicitFreeReusesSlot(t *testing.T) {
	h := newTestHeap(t, 64<<10, 1024, 64)
	a := h.alloc(40)
	if !h.c.Free(unsafe.Pointer(a)) {
		t.Fatal("Free = false, want true")
	}
	if h.c.Free(unsafe.Pointer(a)) {
		t.Error("double Free = true, want false")
	}
	if got := *word(a, 1); got != poisonWord {
		t.Errorf("freed memory not poisoned: %#x", got)
	}
	if b := h.alloc(40); b != a {
		t.Errorf("Alloc after Free = %#x, want reused %#x", b, a)
	}
	h.check()
}

func TestFreeRejectsForeignPointers(t *testing.T) {
	h := newTestHeap(t, 64<<10, 1024, 64)
	obj := h.alloc(64)
	big := h.alloc(5000)
	var local int

	tests := []struct {
		name string
		p    uintptr
	}{
		{name: "nil", p: 0},
		{name: "outside heap", p: uintptr(unsafe.Pointer(&local))},
		{name: "interior of small", p: obj + 8},
		{name: "interior of large", p: big + 2048},
		{name: "unallocated slot", p: obj + 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if h.c.Free(unsafe.Pointer(tt.p)) {
				t.Errorf("Free(%#x) = true, want false", tt.p)
			}
		})
	}
	h.check()
}

func TestCollectKeepsReachableAndFreesGarbage(t *testing.T) {
	h := newTestHeap(t, 256<<10, 1024, 64)

	// Make a list of 200 nodes. Word 0 is the next link and word 1 is a check value.
	const n = 200
	nodes := make([]uintptr, n)
	for i := range nodes {
		nodes[i] = h.alloc(32)
		*word(nodes[i], 1) = uintptr(i * 7)
		if i > 0 {
			*word(nodes[i-1], 0) = nodes[i]
		}
	}
	garbage := h.alloc(4000)

	h.roots[0] = nodes[0]
	h.collect()

	for i, node := range nodes {
		if !h.allocated(node) {
			t.Fatalf("node %d freed while reachable", i)
		}
		if got := *word(node, 1); got != uintptr(i*7) {
			t.Fatalf("node %d value = %d, want %d", i, got, i*7)
		}
	}
	if h.allocated(garbage) {
		t.Error("unreachable large object survived")
	}

	// Cut the list in half.
	*word(nodes[n/2-1], 0) = 0
	h.collect()
	for i, node := range nodes {
		if want := i < n/2; h.allocated(node) != want {
			t.Fatalf("node %d allocated = %v, want %v", i, !want, want)
		}
	}
	if got := h.c.Stats().NumGC; got != 2 {
		t.Errorf("NumGC = %d, want 2", got)
	}
}

func TestInteriorPointerKeepsObjectAlive(t *testing.T) {
	h := newTestHeap(t, 256<<10, 1024, 64)
	small := h.alloc(96)
	large := h.alloc(3 * 1024)
	h.roots[0] = small + 40
	h.roots[1] = large + 2*1024 + 8 // points into a tail page
	h.collect()
	if !h.allocated(small) || !h.allocated(large) {
		t.Fatalf("interior-referenced objects freed: small=%v large=%v", h.allocated(small), h.allocated(large))
	}
}

func TestOnePastEndPointerDoesNotPinNeighbour(t *testing.T) {
	h := newTestHeap(t, 256<<10, 1024, 64)
	stack := h.alloc(4 * 1024) // exact page multiple, like a goroutine stack
	next := h.alloc(4 * 1024)
	h.roots[0] = stack + 4*1024 // saved "top" of the stack
	h.collect()
	if !h.allocated(stack) {
		t.Error("object freed while a one-past-the-end pointer to it exists")
	}
	if h.allocated(next) {
		t.Error("neighbouring object pinned by a one-past-the-end pointer")
	}
}

func TestWorklistOverflowStillMarksEverything(t *testing.T) {
	// A wide tree with a tiny worklist forces the overflow rescan path.
	h := newTestHeap(t, 512<<10, 1024, 2)
	root := h.alloc(1024) // 128 pointer slots on 64-bit, 256 on 32-bit
	slots := uintptr(1024) / wordSize
	children := make([]uintptr, slots)
	for i := range children {
		children[i] = h.alloc(16)
		*word(root, uintptr(i)) = children[i]
		leaf := h.alloc(16)
		*word(children[i], 0) = leaf
	}
	h.roots[0] = root
	h.collect()
	for i, child := range children {
		if !h.allocated(child) || !h.allocated(*word(child, 0)) {
			t.Fatalf("child %d or its leaf freed after worklist overflow", i)
		}
	}
}

func TestEmptyPagesReturnToPool(t *testing.T) {
	h := newTestHeap(t, 128<<10, 1024, 64)
	idle := h.c.Stats().HeapIdle
	for i := 0; i < 500; i++ {
		h.alloc(48)
	}
	if h.c.Stats().HeapIdle >= idle {
		t.Fatal("allocations did not consume pages")
	}
	h.collect()
	if got := h.c.Stats(); got.HeapIdle != idle || got.HeapInuse != 0 {
		t.Errorf("after collect HeapIdle = %d (want %d), HeapInuse = %d (want 0)", got.HeapIdle, idle, got.HeapInuse)
	}
}

func TestExhaustionReturnsNilThenRecovers(t *testing.T) {
	h := newTestHeap(t, 64<<10, 1024, 64)
	for h.c.Alloc(200) != nil {
	}
	if h.c.Alloc(5000) != nil {
		t.Fatal("large Alloc on full heap != nil")
	}
	h.collect()
	if h.c.Alloc(5000) == nil {
		t.Fatal("Alloc after collect = nil")
	}
}

func TestReserveIsNeverScannedOrSwept(t *testing.T) {
	h := newTestHeap(t, 128<<10, 1024, 64)
	mem := h.c.Reserve(3000)
	if len(mem) < 3000 {
		t.Fatalf("Reserve len = %d, want >= 3000", len(mem))
	}
	target := h.alloc(64)
	// The only reference to target is in the reservation, which the collector does not scan.
	// The collector must free target and keep the reservation.
	*(*uintptr)(unsafe.Pointer(&mem[0])) = target
	h.roots[0] = uintptr(unsafe.Pointer(&mem[16])) // interior pointer into the reservation
	h.collect()
	if h.allocated(target) {
		t.Error("object referenced only from a reservation survived")
	}
	if got := h.c.Stats().HeapReserved; got != uint64(len(mem)) {
		t.Errorf("HeapReserved = %d, want %d", got, len(mem))
	}
	if !h.c.Release(unsafe.Pointer(&mem[0])) {
		t.Fatal("Release = false")
	}
	if h.c.Release(unsafe.Pointer(&mem[0])) {
		t.Error("double Release = true")
	}
	h.check()
}

func TestGrow(t *testing.T) {
	span := testSpan(t, 256<<10)
	start := uintptr(unsafe.Pointer(&span[0]))
	limit := start + 256<<10
	meta := MetadataEnd(start, limit, Config{PageSize: 1024, WorklistLen: 64})

	var c Collector
	if err := c.Init(start, meta+4*1024, limit, Config{PageSize: 1024, WorklistLen: 64}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if c.Alloc(8*1024) != nil {
		t.Fatal("8 KiB Alloc fit in a 4-page heap")
	}
	if !c.Grow(meta + 64*1024) {
		t.Fatal("Grow = false")
	}
	if c.Alloc(8*1024) == nil {
		t.Fatal("Alloc after Grow = nil")
	}
	if c.Grow(limit+1<<20) && c.Limit() > limit {
		t.Error("Grow went past limit")
	}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(span)
}

// randomSteps is the length of TestRandomisedAgainstModel. The board sets it lower in
// steps_baremetal_test.go, because each step checks every live object in slow memory.
var randomSteps = 20000

func TestRandomisedAgainstModel(t *testing.T) {
	h := newTestHeap(t, 1<<20, 1024, 16)
	rng := rand.New(rand.NewSource(1))
	var live []liveObject
	for step := 0; step < randomSteps; step++ {
		switch op := rng.Intn(10); {
		case op < 6:
			size := uintptr(1 + rng.Intn(3000))
			p := h.c.Alloc(size)
			if p == nil {
				h.collectLive(live)
				continue
			}
			o := liveObject{addr: uintptr(p), size: size, tag: uintptr(rng.Int63())}
			*word(o.addr, 0) = o.tag
			live = append(live, o)
		case op < 8 && len(live) > 0:
			i := rng.Intn(len(live))
			if !h.c.Free(unsafe.Pointer(live[i].addr)) {
				t.Fatalf("step %d: Free(live object) = false", step)
			}
			live = append(live[:i], live[i+1:]...)
		case op == 8 && len(live) > 0:
			// Drop the object without Free. The next collection must reclaim it.
			i := rng.Intn(len(live))
			live = append(live[:i], live[i+1:]...)
		default:
			h.collectLive(live)
		}
		for _, o := range live {
			if *word(o.addr, 0) != o.tag {
				t.Fatalf("step %d: object %#x corrupted", step, o.addr)
			}
		}
	}
	h.collectLive(live)
	var want uint64
	for _, o := range live {
		want += uint64(h.c.objectSize(o.addr))
	}
	if got := h.c.Stats().HeapInuse; got != want {
		t.Errorf("HeapInuse = %d, want %d", got, want)
	}
}

// liveObject is a model-side record of an object the test keeps alive.
type liveObject struct {
	addr, size, tag uintptr
}

// collectLive runs a cycle with every model-live object as a root.
func (h *testHeap) collectLive(live []liveObject) {
	h.t.Helper()
	h.c.BeginCycle()
	for _, o := range live {
		h.c.MarkPointer(o.addr)
	}
	h.c.FinishMark()
	h.c.Sweep()
	h.check()
}

// TestMarkStackSkipsDeadPart checks that MarkStack keeps the stack object and scans only the part
// above sp. A pointer in the dead part below sp must not keep its object alive.
func TestMarkStackSkipsDeadPart(t *testing.T) {
	h := newTestHeap(t, 64<<10, 1024, 64)
	stack := h.alloc(512)
	dead, live := h.alloc(32), h.alloc(32)
	sp := stack + 256
	*word(stack, 8) = dead  // below sp minus the save area, dead
	*word(stack, 40) = live // above sp, live
	h.c.BeginCycle()
	h.c.MarkStack(sp)
	h.c.FinishMark()
	h.c.Sweep()
	if !getBit(h.c.allocBits, h.c.granuleOf(stack)) || !getBit(h.c.allocBits, h.c.granuleOf(live)) {
		t.Fatal("the stack or the object that its live part points to was freed")
	}
	if getBit(h.c.allocBits, h.c.granuleOf(dead)) {
		t.Error("a pointer below sp kept its object alive")
	}
}
