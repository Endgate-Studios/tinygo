package collector

import "unsafe"

// Set is one GC heap over a primary and an optional secondary span, for example internal
// DRAM and external PSRAM. Pointers between the spans are followed in the two directions.
type Set struct {
	primary   *Collector
	secondary *Collector // nil if there is no secondary span

	// Page region of the secondary span. Both are 0 without a secondary span.
	lo, hi uintptr
}

// NewSet makes a set over two initialised collectors. secondary can be nil.
// The spans must not overlap, also at their growth limits.
func NewSet(primary, secondary *Collector) (Set, error) {
	if primary == nil || primary.nPages == 0 || primary == secondary {
		return Set{}, ErrBadSet
	}
	s := Set{primary: primary}
	if secondary == nil {
		return s, nil
	}
	if secondary.nPages == 0 ||
		primary.pagesStart < secondary.Limit() && secondary.pagesStart < primary.Limit() {
		return Set{}, ErrBadSet
	}
	s.secondary = secondary
	s.lo, s.hi = secondary.Bounds()
	return s, nil
}

// inSecondary reports whether v is in the page region of the secondary span.
func (s *Set) inSecondary(v uintptr) bool {
	return v >= s.lo && v < s.hi
}

// Contains reports whether p is in the page region of a span.
func (s *Set) Contains(p unsafe.Pointer) bool {
	return s.inSecondary(uintptr(p)) || s.primary.Contains(p)
}

// Alloc tries the primary span first and the secondary span when the primary is full.
func (s *Set) Alloc(size uintptr) unsafe.Pointer {
	if p := s.primary.Alloc(size); p != nil || s.secondary == nil {
		return p
	}
	return s.secondary.Alloc(size)
}

// Free gives the object to the span that holds it. See Collector.Free.
func (s *Set) Free(p unsafe.Pointer) bool {
	if s.inSecondary(uintptr(p)) {
		return s.secondary.Free(p)
	}
	return s.primary.Free(p)
}

// Reserve takes unscanned pages from the primary span only.
func (s *Set) Reserve(size uintptr) []byte {
	return s.primary.Reserve(size)
}

// Release gives back a reservation from Reserve.
func (s *Set) Release(p unsafe.Pointer) bool {
	return s.primary.Release(p)
}

// BeginCycle starts a collection cycle on each span.
func (s *Set) BeginCycle() {
	s.primary.BeginCycle()
	if s.secondary != nil {
		s.secondary.BeginCycle()
	}
}

// MarkRange treats each aligned word in [start, end) as a possible pointer into a span.
func (s *Set) MarkRange(start, end uintptr) {
	// One unsigned compare per span rejects a word outside the heap before any call.
	plo, pspan := s.primary.pagesStart, s.primary.pagesEnd-s.primary.pagesStart
	slo, sspan := s.lo, s.hi-s.lo
	for a := alignUp(start, wordSize); a+wordSize <= end; a += wordSize {
		v := *(*uintptr)(unsafe.Pointer(a))
		switch {
		case v-plo < pspan:
			s.primary.MarkPointer(v)
		case v-slo < sspan:
			s.secondary.MarkPointer(v)
		}
	}
}

// MarkPointer marks the object that v points into, in the span that holds it.
func (s *Set) MarkPointer(v uintptr) {
	if s.inSecondary(v) {
		s.secondary.MarkPointer(v)
		return
	}
	s.primary.MarkPointer(v)
}

// MarkStack marks the goroutine stack that holds sp, in the span that holds it. See Collector.MarkStack.
func (s *Set) MarkStack(sp uintptr) {
	if s.inSecondary(sp) {
		s.secondary.MarkStack(sp)
		return
	}
	s.primary.MarkStack(sp)
}

// FinishMark scans the queued objects of the two spans until neither span has work.
// A scan in one span can queue objects in the other span.
func (s *Set) FinishMark() {
	for {
		s.drain(s.primary)
		if s.secondary != nil {
			s.drain(s.secondary)
		}
		switch {
		case s.primary.top > 0:
		case s.primary.overflow:
			s.rescan(s.primary)
		case s.secondary != nil && s.secondary.overflow:
			s.rescan(s.secondary)
		default:
			return
		}
	}
}

// drain does the same as Collector.drain, but the scan marks objects in the two spans.
func (s *Set) drain(c *Collector) {
	for c.top > 0 {
		c.top -= 2
		start, end := c.worklist[c.top], c.worklist[c.top+1]
		c.worklist[c.top], c.worklist[c.top+1] = 0, 0
		if end-start > scanChunk {
			c.push(start+scanChunk, end)
			end = start + scanChunk
		}
		s.MarkRange(start, end)
	}
}

// setScan is the forEachObject context for a rescan after a worklist overflow.
type setScan struct {
	s *Set
	c *Collector
}

// rescan scans the marked objects of c again, to find the ranges that an overflow dropped.
func (s *Set) rescan(c *Collector) {
	c.overflow = false
	forEachObject(c, setScan{s: s, c: c}, func(x setScan, obj, size uintptr) {
		if getBit(x.c.markBits, x.c.granuleOf(obj)) {
			x.s.MarkRange(obj, obj+size)
			x.s.drain(x.s.primary)
			if x.s.secondary != nil {
				x.s.drain(x.s.secondary)
			}
		}
	})
}

// Sweep frees the unmarked objects of each span.
func (s *Set) Sweep() {
	s.primary.Sweep()
	if s.secondary != nil {
		s.secondary.Sweep()
	}
}

// Stats returns the sum of the span counters. NumGC counts each set cycle one time.
func (s *Set) Stats() Stats {
	a := s.primary.Stats()
	if s.secondary == nil {
		return a
	}
	b := s.secondary.Stats()
	a.HeapSys += b.HeapSys
	a.HeapInuse += b.HeapInuse
	a.HeapIdle += b.HeapIdle
	a.HeapReserved += b.HeapReserved
	a.MetaSys += b.MetaSys
	a.TotalAlloc += b.TotalAlloc
	a.Mallocs += b.Mallocs
	a.Frees += b.Frees
	return a
}

// Check validates the invariants of each span.
func (s *Set) Check() error {
	if err := s.primary.Check(); err != nil || s.secondary == nil {
		return err
	}
	return s.secondary.Check()
}
