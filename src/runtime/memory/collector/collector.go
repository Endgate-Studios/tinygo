// Package collector is a conservative, non-moving mark-sweep collector with size classes.
// It does not know the TinyGo runtime. NOTES.md (Collector design) has the layout and rules.
package collector

import "unsafe"

const (
	wordSize = unsafe.Sizeof(uintptr(0))

	// granule is the allocation unit and the bitmap resolution. 16 bytes satisfies the
	// TinyGo heap alignment (func align in src/runtime/arch_tinygowasm.go and arch_xtensa.go).
	granule = 16

	// MinPageSize is the smallest accepted Config.PageSize.
	MinPageSize = 256
)

// classSizes are the small object size classes. Init uses only classes of at most
// half a page, so each class page holds two or more objects.
var classSizes = [...]uintptr{16, 32, 48, 64, 96, 128, 192, 256, 384, 512, 768, 1024, 1536, 2048}

const numClasses = len(classSizes)

// Page kinds. Each page has one kind byte.
const (
	kindFree       uint8 = iota // unused, available to any request
	kindLargeHead               // first page of a large object
	kindLargeTail               // continuation page of a large object
	kindManualHead              // first page of an unscanned reservation
	kindManualTail              // continuation page of an unscanned reservation
	kindSmall      uint8 = 16   // kindSmall + class index
)

// poisonWord fills freed memory when Config.Poison is set. It makes use after free visible.
const poisonWord = uintptr(0xDEADBEEFDEADBEEF & uint64(^uintptr(0)))

// Config tunes a collector.
type Config struct {
	// PageSize is a power of two of at least MinPageSize. Small pages waste less on small heaps.
	// Use 1 KiB on MCUs and 4 KiB on wasm.
	PageSize uintptr

	// WorklistLen is the number of ranges in the mark stack. A small value is safe but slower,
	// because an overflow makes the collector scan the marked objects again.
	WorklistLen uintptr

	// Poison fills freed memory with poisonWord.
	Poison bool
}

// Stats is a snapshot of collector counters.
type Stats struct {
	HeapSys      uint64 // bytes of page memory currently usable
	HeapInuse    uint64 // bytes in allocated objects (slot sizes)
	HeapIdle     uint64 // bytes in free pages
	HeapReserved uint64 // bytes in unscanned reservations
	MetaSys      uint64 // bytes used by collector metadata
	TotalAlloc   uint64 // cumulative requested bytes
	Mallocs      uint64
	Frees        uint64
	NumGC        uint32
}

// Collector is a mark-sweep heap over one span. Call Init before use, because a constructor
// would need the heap that it makes. NOTES.md (Struct layout) explains the field order.
type Collector struct {
	stats Stats

	// The mark path reads these fields for each scanned word.
	pagesStart uintptr   // address of page 0
	pagesEnd   uintptr   // pagesStart + nPages*PageSize
	pageShift  uintptr   // log2(PageSize)
	kinds      []uint8   // kind of each page
	runs       []uint32  // run length at a head page, head index at a tail page
	allocBits  []uint32  // set at the granule where an allocated object starts
	markBits   []uint32  // set at the granule where a marked object starts
	worklist   []uintptr // explicit mark stack of [start, end) pairs
	top        uintptr   // worklist depth in words (two per entry)

	// Allocation path.
	maxSmall  uintptr             // largest size served by a class
	freeLists [numClasses]uintptr // free-slot list head per class
	slots     [numClasses]uintptr // slots per page per class
	hint      uintptr             // no free page below this index
	freePages uintptr
	nPages    uintptr // pages currently usable
	maxPages  uintptr // pages the metadata was sized for

	cfg       Config
	zeroSized [granule]byte // returned for zero-size allocations
	overflow  bool          // worklist overflowed during this mark
}

// MetadataEnd returns the end of the metadata for a span at start that can grow to limit.
// Use it to find how much memory to commit before Init, for example with wasm memory.grow.
func MetadataEnd(start, limit uintptr, cfg Config) uintptr {
	base := alignUp(start, granule)
	if limit <= base || cfg.PageSize == 0 {
		return base
	}
	maxPages := (limit - base) / cfg.PageSize
	granules := maxPages * cfg.PageSize / granule
	bitmapBytes := alignUp(granules, 32) / 8

	end := base
	end += maxPages                       // kinds
	end = alignUp(end, 4) + 4*maxPages    // runs
	end = alignUp(end, 4) + 2*bitmapBytes // alloc + mark bitmaps
	end = alignUp(end, wordSize) + 2*wordSize*cfg.WorklistLen
	return alignUp(end, granule)
}

// Init sets up the collector over [start, end), with metadata for growth up to limit.
// Use limit == end for a fixed heap. Alloc zeroes page memory, so Init does not.
func (c *Collector) Init(start, end, limit uintptr, cfg Config) error {
	if cfg.PageSize < MinPageSize || cfg.PageSize&(cfg.PageSize-1) != 0 {
		return ErrBadPageSize
	}
	if cfg.WorklistLen == 0 {
		cfg.WorklistLen = 64
	}
	if limit < end {
		limit = end
	}

	// Do not use a composite literal here. TinyGo puts it on the heap, which does not exist yet.
	// See NOTES.md (Bugs found while testing) and the -print-allocs check in README.md.
	clear(unsafe.Slice((*byte)(unsafe.Pointer(c)), unsafe.Sizeof(*c)))
	c.cfg = cfg
	for c.pageShift = 0; uintptr(1)<<c.pageShift < cfg.PageSize; c.pageShift++ {
	}
	base := alignUp(start, granule)
	metaEnd := MetadataEnd(start, limit, cfg)
	if limit <= base || metaEnd >= end || end-metaEnd < cfg.PageSize {
		return ErrHeapTooSmall
	}
	sized := (limit - base) / cfg.PageSize

	c.pagesStart = metaEnd
	c.maxPages = (limit - metaEnd) / cfg.PageSize
	c.nPages = (end - metaEnd) / cfg.PageSize
	c.pagesEnd = c.pageAddr(c.nPages)

	granules := sized * cfg.PageSize / granule
	bitmapWords := alignUp(granules, 32) / 32

	p := base
	c.kinds = unsafe.Slice((*uint8)(unsafe.Pointer(p)), sized)
	p = alignUp(p+sized, 4)
	c.runs = unsafe.Slice((*uint32)(unsafe.Pointer(p)), sized)
	p = alignUp(p+4*sized, 4)
	c.allocBits = unsafe.Slice((*uint32)(unsafe.Pointer(p)), bitmapWords)
	p += 4 * bitmapWords
	c.markBits = unsafe.Slice((*uint32)(unsafe.Pointer(p)), bitmapWords)
	p = alignUp(p+4*bitmapWords, wordSize)
	c.worklist = unsafe.Slice((*uintptr)(unsafe.Pointer(p)), 2*cfg.WorklistLen)

	clear(unsafe.Slice((*byte)(unsafe.Pointer(base)), metaEnd-base))

	for i, size := range classSizes {
		if size <= cfg.PageSize/2 {
			c.maxSmall = size
			c.slots[i] = cfg.PageSize / size
		}
	}
	c.freePages = c.nPages
	c.stats.MetaSys = uint64(metaEnd - start)
	return nil
}

// Grow extends the heap to end, but not past the Init limit. The memory must be accessible first.
// It reports whether it added pages.
func (c *Collector) Grow(end uintptr) bool {
	if end <= c.pagesStart {
		return false
	}
	n := (end - c.pagesStart) / c.cfg.PageSize
	if n > c.maxPages {
		n = c.maxPages
	}
	if n <= c.nPages {
		return false
	}
	c.freePages += n - c.nPages
	c.nPages = n
	c.pagesEnd = c.pageAddr(n)
	return true
}

// Bounds returns the page region [start, end).
func (c *Collector) Bounds() (start, end uintptr) {
	return c.pagesStart, c.pagesEnd
}

// Contains reports whether p is in the page region.
func (c *Collector) Contains(p unsafe.Pointer) bool {
	return uintptr(p) >= c.pagesStart && uintptr(p) < c.pagesEnd
}

// usedMarkBits returns the mark bitmap words for the usable pages. The bitmap covers the
// growth limit, so a full clear on each cycle would cost time for the limit, not the heap.
func (c *Collector) usedMarkBits() []uint32 {
	granules := c.nPages * c.cfg.PageSize / granule
	return c.markBits[:alignUp(granules, 32)/32]
}

// Limit returns the highest address the heap can grow to.
func (c *Collector) Limit() uintptr {
	return c.pagesStart + c.maxPages*c.cfg.PageSize
}

// Alloc returns size zeroed bytes with 16-byte alignment, or nil if the heap is full.
// On nil, the caller decides whether to collect or grow.
func (c *Collector) Alloc(size uintptr) unsafe.Pointer {
	if size == 0 {
		return unsafe.Pointer(&c.zeroSized)
	}
	var obj, objSize uintptr
	if size <= c.maxSmall {
		cls := classFor(size)
		if c.freeLists[cls] == 0 && !c.refill(cls) {
			return nil
		}
		obj = c.freeLists[cls]
		c.freeLists[cls] = *(*uintptr)(unsafe.Pointer(obj))
		objSize = classSizes[cls]
	} else {
		// One byte of slack keeps a one-past-the-end pointer in this object. Dead stack tops
		// (src/internal/task/task_asyncify.go) otherwise pin the next run (see NOTES.md).
		n := pagesFor(size+1, c.cfg.PageSize)
		if n == 0 {
			return nil // overflow
		}
		head, ok := c.takeRun(n, kindLargeHead, kindLargeTail)
		if !ok {
			return nil
		}
		obj = c.pageAddr(head)
		objSize = n * c.cfg.PageSize
	}
	setBit(c.allocBits, c.granuleOf(obj))
	clear(unsafe.Slice((*byte)(unsafe.Pointer(obj)), objSize))
	c.stats.Mallocs++
	c.stats.TotalAlloc += uint64(size)
	c.stats.HeapInuse += uint64(objSize)
	return unsafe.Pointer(obj)
}

// Free releases the object that starts at p. It returns false and does nothing
// if p is not the start of a live object.
func (c *Collector) Free(p unsafe.Pointer) bool {
	obj := uintptr(p)
	start, end := c.Bounds()
	if obj < start || obj >= end {
		return false
	}
	page := (obj - start) >> c.pageShift
	pageAddr := c.pageAddr(page)
	switch k := c.kinds[page]; {
	case k >= kindSmall:
		cls := k - kindSmall
		size := classSizes[cls]
		off := obj - pageAddr
		if off%size != 0 || off/size >= c.slots[cls] {
			return false
		}
		g := c.granuleOf(obj)
		if !getBit(c.allocBits, g) {
			return false
		}
		clearBit(c.allocBits, g)
		c.poison(obj, size)
		*(*uintptr)(unsafe.Pointer(obj)) = c.freeLists[cls]
		c.freeLists[cls] = obj
		c.stats.HeapInuse -= uint64(size)
	case k == kindLargeHead:
		g := c.granuleOf(obj)
		if obj != pageAddr || !getBit(c.allocBits, g) {
			return false
		}
		clearBit(c.allocBits, g)
		n := uintptr(c.runs[page])
		c.poison(obj, n*c.cfg.PageSize)
		c.releaseRun(page)
		c.stats.HeapInuse -= uint64(n * c.cfg.PageSize)
	default:
		return false
	}
	c.stats.Frees++
	return true
}

// Reserve takes a zeroed run of at least size bytes that the collector never scans or sweeps.
// It returns nil if no run is free.
func (c *Collector) Reserve(size uintptr) []byte {
	n := pagesFor(size, c.cfg.PageSize)
	if n == 0 {
		return nil
	}
	head, ok := c.takeRun(n, kindManualHead, kindManualTail)
	if !ok {
		return nil
	}
	mem := unsafe.Slice((*byte)(unsafe.Pointer(c.pageAddr(head))), n*c.cfg.PageSize)
	clear(mem)
	c.stats.HeapReserved += uint64(len(mem))
	return mem
}

// Release gives back a reservation from Reserve. It returns false if p is not
// the start of a reservation.
func (c *Collector) Release(p unsafe.Pointer) bool {
	obj := uintptr(p)
	start, end := c.Bounds()
	if obj < start || obj >= end {
		return false
	}
	page := (obj - start) >> c.pageShift
	if c.kinds[page] != kindManualHead || c.pageAddr(page) != obj {
		return false
	}
	c.stats.HeapReserved -= uint64(uintptr(c.runs[page]) * c.cfg.PageSize)
	c.releaseRun(page)
	return true
}

// Stats returns a snapshot of the collector counters.
func (c *Collector) Stats() Stats {
	s := c.stats
	s.HeapSys = uint64(c.nPages * c.cfg.PageSize)
	s.HeapIdle = uint64(c.freePages * c.cfg.PageSize)
	return s
}

func (c *Collector) pageAddr(page uintptr) uintptr {
	return c.pagesStart + page*c.cfg.PageSize
}

func (c *Collector) granuleOf(addr uintptr) uintptr {
	return (addr - c.pagesStart) / granule
}

// refill turns one free page into slots of class cls.
func (c *Collector) refill(cls int) bool {
	page, ok := c.takeRun(1, kindSmall+uint8(cls), kindSmall+uint8(cls))
	if !ok {
		return false
	}
	c.pushSlots(page, cls)
	return true
}

// pushSlots puts each unallocated slot of a class page on the free list,
// with the lowest address first.
func (c *Collector) pushSlots(page uintptr, cls int) {
	size := classSizes[cls]
	base := c.pageAddr(page)
	for i := c.slots[cls]; i > 0; i-- {
		slot := base + (i-1)*size
		if getBit(c.allocBits, c.granuleOf(slot)) {
			continue
		}
		*(*uintptr)(unsafe.Pointer(slot)) = c.freeLists[cls]
		c.freeLists[cls] = slot
	}
}

// takeRun finds the first run of n free pages and claims it.
func (c *Collector) takeRun(n uintptr, head, tail uint8) (uintptr, bool) {
	if n > c.freePages {
		return 0, false
	}
	run := uintptr(0)
	for p := c.hint; p < c.nPages; p++ {
		if c.kinds[p] != kindFree {
			run = 0
			continue
		}
		run++
		if run < n {
			continue
		}
		first := p + 1 - n
		c.kinds[first] = head
		c.runs[first] = uint32(n)
		for q := first + 1; q <= p; q++ {
			c.kinds[q] = tail
			c.runs[q] = uint32(first)
		}
		c.freePages -= n
		if first == c.hint {
			c.hint = p + 1
		}
		return first, true
	}
	return 0, false
}

// releaseRun frees the run of pages starting at head.
func (c *Collector) releaseRun(head uintptr) {
	n := uintptr(1)
	if k := c.kinds[head]; k < kindSmall {
		n = uintptr(c.runs[head])
	}
	for p := head; p < head+n; p++ {
		c.kinds[p] = kindFree
		c.runs[p] = 0
	}
	c.freePages += n
	if head < c.hint {
		c.hint = head
	}
}

func (c *Collector) poison(addr, size uintptr) {
	if !c.cfg.Poison {
		return
	}
	for a := addr; a+wordSize <= addr+size; a += wordSize {
		*(*uintptr)(unsafe.Pointer(a)) = poisonWord
	}
}

func classFor(size uintptr) int {
	for i, s := range classSizes {
		if size <= s {
			return i
		}
	}
	return numClasses - 1
}

func pagesFor(size, pageSize uintptr) uintptr {
	if size > ^uintptr(0)-pageSize {
		return 0
	}
	return (size + pageSize - 1) / pageSize
}

func alignUp(v, a uintptr) uintptr {
	return (v + a - 1) &^ (a - 1)
}

func getBit(bits []uint32, i uintptr) bool { return bits[i/32]&(1<<(i%32)) != 0 }
func setBit(bits []uint32, i uintptr)      { bits[i/32] |= 1 << (i % 32) }
func clearBit(bits []uint32, i uintptr)    { bits[i/32] &^= 1 << (i % 32) }

// bitWord returns the bitmap word of bit i with no bounds check. Callers must know that i is in
// the bitmap, for example because objectAt found the object in the page region.
func bitWord(bits []uint32, i uintptr) *uint32 {
	return (*uint32)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(bits)), i/32*4))
}
