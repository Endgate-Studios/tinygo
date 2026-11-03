package collector

import (
	"math/bits"
	"unsafe"
)

// BeginCycle clears all mark bits and starts a collection cycle.
func (c *Collector) BeginCycle() {
	clear(c.usedMarkBits())
	c.top = 0
	c.overflow = false
	c.stats.NumGC++
}

// MarkRange treats each aligned word in [start, end) as a possible pointer
// and marks the object that it points into.
func (c *Collector) MarkRange(start, end uintptr) {
	// One unsigned compare rejects a word outside the page region before any call.
	lo, span := c.pagesStart, c.pagesEnd-c.pagesStart
	for a := alignUp(start, wordSize); a+wordSize <= end; a += wordSize {
		if v := *(*uintptr)(unsafe.Pointer(a)); v-lo < span {
			c.MarkPointer(v)
		}
	}
}

// MarkPointer marks the object that v points into, if there is one. An interior
// pointer keeps the full object alive. Each new marked object goes on the scan queue.
func (c *Collector) MarkPointer(v uintptr) {
	obj, ok := c.objectAt(v)
	if !ok {
		return
	}
	g := c.granuleOf(obj)
	bit := uint32(1) << (g % 32)
	mark := bitWord(c.markBits, g)
	if *bitWord(c.allocBits, g)&bit == 0 || *mark&bit != 0 {
		return
	}
	*mark |= bit
	if c.kinds[(obj-c.pagesStart)>>c.pageShift] == kindBytesHead {
		return
	}
	c.push(obj, obj+c.objectSize(obj))
}

// scanChunk is the maximum scan length before the children of an object get a turn.
// Without it, wide objects such as pointer arrays push all children and overflow the worklist.
const scanChunk = 64 * wordSize

// stackSaveArea is the register save area below a stack pointer. On Xtensa, a windowed frame keeps
// the a0 to a3 spill of its caller in the 16 bytes below its stack pointer (Xtensa ISA RM, 8.1.5).
const stackSaveArea = 16

// MarkStack marks the goroutine stack object that holds sp, and queues only its live part,
// from sp to the object end. Old values below sp would otherwise keep garbage alive.
func (c *Collector) MarkStack(sp uintptr) {
	obj, ok := c.objectAt(sp)
	if !ok {
		return
	}
	g := c.granuleOf(obj)
	if !getBit(c.allocBits, g) || getBit(c.markBits, g) {
		return
	}
	setBit(c.markBits, g)
	start := max(obj, sp-stackSaveArea)
	c.push(start, obj+c.objectSize(obj))
}

func (c *Collector) push(start, end uintptr) {
	if c.top+2 > uintptr(len(c.worklist)) {
		// FinishMark finds the dropped range again when it scans the marked objects.
		c.overflow = true
		return
	}
	c.worklist[c.top] = start
	c.worklist[c.top+1] = end
	c.top += 2
}

// FinishMark scans queued objects until all reachable objects are marked. It does not recurse.
// After a worklist overflow, it scans the marked objects again until no new marks occur.
func (c *Collector) FinishMark() {
	for {
		c.drain()
		if !c.overflow {
			return
		}
		c.overflow = false
		forEachObject(c, c, func(c *Collector, obj, size uintptr) {
			if getBit(c.markBits, c.granuleOf(obj)) {
				c.MarkRange(obj, obj+size)
				c.drain()
			}
		})
	}
}

// Sweep frees each unmarked object, releases empty pages and builds the free lists again.
func (c *Collector) Sweep() {
	c.freeLists = [numClasses]uintptr{}
	var inuse uint64
	for page := uintptr(0); page < c.nPages; {
		switch k := c.kinds[page]; {
		case k >= kindSmall:
			inuse += c.sweepSmall(page, int(k-kindSmall))
			page++
		case k == kindLargeHead || k == kindBytesHead:
			n := uintptr(c.runs[page])
			obj := c.pageAddr(page)
			g := c.granuleOf(obj)
			if getBit(c.markBits, g) {
				inuse += uint64(n * c.cfg.PageSize)
			} else {
				clearBit(c.allocBits, g)
				c.poison(obj, n*c.cfg.PageSize)
				c.releaseRun(page)
				c.stats.Frees++
			}
			page += n
		case k == kindManualHead:
			page += uintptr(c.runs[page])
		default:
			page++
		}
	}
	c.stats.HeapInuse = inuse
}

// sweepSmall sweeps one class page and returns the bytes still in use.
func (c *Collector) sweepSmall(page uintptr, cls int) uint64 {
	size := classSizes[cls]
	live := uintptr(0)
	// Alloc and mark bits are set only where an object starts, so a word at a time gives the live
	// objects (alloc and mark) and the dead ones (alloc and not mark). A page can end mid word.
	g0 := c.granuleOf(c.pageAddr(page))
	g1 := g0 + c.cfg.PageSize/granule
	for g := g0; g < g1; {
		w, lo := g/32, g%32
		n := min(32-lo, g1-g)
		mask := ^uint32(0)
		if n < 32 {
			mask = (1<<n - 1) << lo
		}
		a, m := c.allocBits[w]&mask, c.markBits[w]&mask
		live += uintptr(bits.OnesCount32(a & m))
		if dead := a &^ m; dead != 0 {
			c.allocBits[w] &^= dead
			c.stats.Frees += uint64(bits.OnesCount32(dead))
			for d := dead; d != 0 && c.cfg.Poison; d &= d - 1 {
				c.poison(c.pagesStart+(w*32+uintptr(bits.TrailingZeros32(d)))*granule, size)
			}
		}
		g += n
	}
	if live == 0 {
		c.releaseRun(page)
		return 0
	}
	c.pushSlots(page, cls)
	return uint64(live * size)
}

// drain scans scanChunk bytes of a range and puts the rest below its new children, depth first.
// It zeroes each popped entry so that no stale address stays in memory.
func (c *Collector) drain() {
	for c.top > 0 {
		c.top -= 2
		start, end := c.worklist[c.top], c.worklist[c.top+1]
		c.worklist[c.top], c.worklist[c.top+1] = 0, 0
		if end-start > scanChunk {
			c.push(start+scanChunk, end) // cannot overflow, because the pop freed a slot
			end = start + scanChunk
		}
		c.MarkRange(start, end)
	}
}

// objectAt maps any address to the start of the object slot it falls in.
// It does not check the allocation bit.
func (c *Collector) objectAt(v uintptr) (uintptr, bool) {
	if v < c.pagesStart || v >= c.pagesEnd {
		return 0, false
	}
	page := (v - c.pagesStart) >> c.pageShift
	base := c.pageAddr(page)
	switch k := c.kinds[page]; {
	case k >= kindSmall:
		cls := k - kindSmall
		idx := (v - base) / classSizes[cls]
		if idx >= c.slots[cls] {
			return 0, false // tail waste at the end of the page
		}
		return base + idx*classSizes[cls], true
	case k == kindLargeHead || k == kindBytesHead:
		return base, true
	case k == kindLargeTail || k == kindBytesTail:
		return c.pageAddr(uintptr(c.runs[page])), true
	default:
		return 0, false // free pages and unscanned reservations
	}
}

// objectSize returns the slot size of the object starting at obj.
func (c *Collector) objectSize(obj uintptr) uintptr {
	page := (obj - c.pagesStart) >> c.pageShift
	if k := c.kinds[page]; k >= kindSmall {
		return classSizes[k-kindSmall]
	}
	return uintptr(c.runs[page]) * c.cfg.PageSize
}

// forEachObject calls fn with ctx for each allocated object of c that may hold pointers. fn is not a closure,
// so that the call does not allocate.
func forEachObject[T any](c *Collector, ctx T, fn func(ctx T, obj, size uintptr)) {
	for page := uintptr(0); page < c.nPages; {
		switch k := c.kinds[page]; {
		case k >= kindSmall:
			size := classSizes[k-kindSmall]
			base := c.pageAddr(page)
			for i := uintptr(0); i < c.slots[k-kindSmall]; i++ {
				if slot := base + i*size; getBit(c.allocBits, c.granuleOf(slot)) {
					fn(ctx, slot, size)
				}
			}
			page++
		case k == kindLargeHead:
			n := uintptr(c.runs[page])
			if obj := c.pageAddr(page); getBit(c.allocBits, c.granuleOf(obj)) {
				fn(ctx, obj, n*c.cfg.PageSize)
			}
			page += n
		case k == kindManualHead || k == kindBytesHead:
			page += uintptr(c.runs[page])
		default:
			page++
		}
	}
}
