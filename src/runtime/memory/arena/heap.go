// Package arena has allocators over memory that the GC does not scan.
// See README.md for the memory sources and the safety rule.
package arena

import (
	"sync"
	"unsafe"
)

const (
	wordSize = unsafe.Sizeof(uintptr(0))

	// DefaultAlign is the alignment of each Heap block payload. It satisfies
	// max_align_t on 32-bit targets and is 16 on 64-bit hosts.
	DefaultAlign = 2 * wordSize

	unit       = DefaultAlign // block sizes are multiples of unit
	allocated  = uintptr(1)   // low bit of a boundary tag
	minBlock   = 2 * unit     // header + next + prev + footer
	poisonByte = 0xDE
)

// HeapStats is a snapshot of Heap counters.
type HeapStats struct {
	Allocs      uint64
	Frees       uint64
	Failures    uint64 // Alloc calls that returned nil
	FreeBlocks  uint64
	AllocBlocks uint64
	Size        uintptr // usable bytes, including boundary tags
	InUse       uintptr // bytes in allocated blocks, including tags
	Peak        uintptr // highest InUse seen
	Free        uintptr // bytes in free blocks
	LargestFree uintptr // payload bytes of the largest free block
}

// MaxSpans is the number of spans that a Heap can hold, the first one and the ones
// that Grow adds.
const MaxSpans = 8

// Heap is a first-fit malloc and free allocator with boundary tags over one or more
// spans. It splits blocks when it allocates and joins adjacent free blocks when it
// frees. Blocks never join across spans, because each span ends in used sentinels.
type Heap struct {
	stats    HeapStats // first, for 8-byte alignment
	spans    [MaxSpans]span
	nspans   int
	freeList uintptr // first free block, 0 if none
	heapOptions
}

type span struct {
	mem    []byte  // keeps the memory alive under a precise GC
	lo, hi uintptr // first block, epilogue header
}

// HeapOption configures a Heap.
type HeapOption func(*heapOptions) error

type heapOptions struct {
	locker sync.Locker
	poison bool
}

// WithLocker makes every Heap operation hold l. sync.Mutex is sufficient between goroutines.
// On an MCU where interrupt code shares the heap, use an interrupt-safe critical section.
func WithLocker(l sync.Locker) HeapOption {
	return func(o *heapOptions) error {
		if l == nil {
			return ErrNilLocker
		}
		o.locker = l
		return nil
	}
}

// WithPoison fills freed payloads with 0xDE to make use after free visible.
// Heap always detects double frees, with or without poison.
func WithPoison(on bool) HeapOption {
	return func(o *heapOptions) error {
		o.poison = on
		return nil
	}
}

// NewHeap makes a Heap over mem. The Heap owns mem, and mem must stay alive
// while the Heap is in use.
func NewHeap(mem []byte, opts ...HeapOption) (*Heap, error) {
	h := &Heap{}
	if err := h.Init(mem, opts...); err != nil {
		return nil, err
	}
	return h, nil
}

// Init is NewHeap without an allocation, for -gc=none. h is memory that the
// caller owns, for example a global. Call Init before you use h.
func (h *Heap) Init(mem []byte, opts ...HeapOption) error {
	*h = Heap{heapOptions: heapOptions{locker: noLock{}, poison: debugPoison}}
	for _, opt := range opts {
		if err := opt(&h.heapOptions); err != nil {
			return err
		}
	}
	if len(mem) == 0 {
		return ErrArenaTooSmall
	}
	return h.addSpan(mem)
}

// Grow adds mem to the heap as a new span. The Heap owns mem, and mem must stay
// alive while the Heap is in use. A heap cannot shrink, because callers can hold
// blocks in every span.
func (h *Heap) Grow(mem []byte) error {
	h.locker.Lock()
	defer h.locker.Unlock()
	return h.addSpan(mem)
}

func (h *Heap) addSpan(mem []byte) error {
	if h.nspans == MaxSpans {
		return ErrTooManySpans
	}
	if len(mem) == 0 {
		return ErrArenaTooSmall
	}
	start := uintptr(unsafe.Pointer(unsafe.SliceData(mem)))
	end := start + uintptr(len(mem))
	// Blocks start at unit minus W so that each payload at block plus W is unit aligned.
	// The word before the first block holds the prologue footer.
	first := alignUp(start+2*wordSize, unit) - wordSize
	last := alignDown(end, unit) - wordSize
	if last <= first || last-first < minBlock {
		return ErrArenaTooSmall
	}
	h.spans[h.nspans] = span{mem: mem, lo: first, hi: last}
	h.nspans++
	setWord(first-wordSize, allocated) // prologue footer
	setWord(last, allocated)           // epilogue header
	h.setTags(first, last-first, false)
	h.pushFree(first)
	h.stats.Size += last - first
	h.stats.Free += last - first
	return nil
}

// spanOf returns the span that holds address b, or nil.
func (h *Heap) spanOf(b uintptr) *span {
	for i := range h.nspans {
		if s := &h.spans[i]; b >= s.lo && b < s.hi {
			return s
		}
	}
	return nil
}

// FromRange returns [lo, hi) as a byte slice, for a linker region outside the GC ranges.
// Declare the bounds with //go:extern and give their addresses.
func FromRange(lo, hi uintptr) []byte {
	if hi <= lo {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(lo)), hi-lo)
}

// Alloc returns size zeroed bytes aligned to align, or nil if no free block fits.
// An align of 0 selects DefaultAlign.
func (h *Heap) Alloc(size, align uintptr) unsafe.Pointer {
	h.locker.Lock()
	defer h.locker.Unlock()
	return h.alloc(size, align, true)
}

// Calloc allocates n zeroed elements of size bytes.
func (h *Heap) Calloc(n, size uintptr) unsafe.Pointer {
	h.locker.Lock()
	defer h.locker.Unlock()
	if size != 0 && n > ^uintptr(0)/size {
		h.stats.Failures++
		return nil
	}
	return h.alloc(n*size, 0, true)
}

// Free releases p. It panics on a double free or on a pointer from a different source,
// because to continue would corrupt the heap.
func (h *Heap) Free(p unsafe.Pointer) {
	if p == nil {
		return
	}
	h.locker.Lock()
	defer h.locker.Unlock()
	h.free(uintptr(p))
}

// Realloc resizes p to size bytes, in place if possible, and zeroes the new bytes.
// Realloc(nil, n) is Alloc. Realloc(p, 0) frees p. A failure returns nil and keeps p.
func (h *Heap) Realloc(p unsafe.Pointer, size uintptr) unsafe.Pointer {
	if p == nil {
		return h.Alloc(size, 0)
	}
	h.locker.Lock()
	defer h.locker.Unlock()
	if size == 0 {
		h.free(uintptr(p))
		return nil
	}

	b := uintptr(p) - wordSize
	h.checkAllocated(b)
	have := blockSize(b)
	oldPayload := have - 2*wordSize
	need, ok := blockFor(size)
	if !ok {
		h.stats.Failures++
		return nil
	}

	if need <= have {
		h.shrink(b, need)
		return p
	}
	if next := b + have; !isAllocated(next) && have+blockSize(next) >= need {
		h.removeFree(next)
		h.stats.Free -= blockSize(next)
		h.stats.InUse += blockSize(next)
		h.setTags(b, have+blockSize(next), true)
		h.shrink(b, need)
		clearBytes(uintptr(p)+oldPayload, size-oldPayload)
		h.notePeak()
		return p
	}

	q := h.alloc(size, 0, false)
	if q == nil {
		return nil
	}
	copyBytes(uintptr(q), uintptr(p), oldPayload)
	clearBytes(uintptr(q)+oldPayload, size-oldPayload)
	h.free(uintptr(p))
	return q
}

// Contains reports whether p is in a span of the heap. It does not check that
// p is the start of a block.
func (h *Heap) Contains(p unsafe.Pointer) bool {
	h.locker.Lock()
	defer h.locker.Unlock()
	return h.spanOf(uintptr(p)) != nil
}

// Size gives the payload size of the block at p, which can be more than the
// size that Alloc asked for. It panics if p is not an allocated block.
func (h *Heap) Size(p unsafe.Pointer) uintptr {
	h.locker.Lock()
	defer h.locker.Unlock()
	b := uintptr(p) - wordSize
	h.checkAllocated(b)
	return blockSize(b) - 2*wordSize
}

// Stats returns a snapshot of the counters. It walks the heap to get LargestFree
// and the block counts.
func (h *Heap) Stats() HeapStats {
	h.locker.Lock()
	defer h.locker.Unlock()
	s := h.stats
	for i := range h.nspans {
		sp := &h.spans[i]
		for b := sp.lo; b < sp.hi; b += blockSize(b) {
			if isAllocated(b) {
				s.AllocBlocks++
				continue
			}
			s.FreeBlocks++
			if payload := blockSize(b) - 2*wordSize; payload > s.LargestFree {
				s.LargestFree = payload
			}
		}
	}
	return s
}

// Check validates all blocks and the free list and returns the first error.
// Use it in tests and debug builds.
func (h *Heap) Check() error {
	h.locker.Lock()
	defer h.locker.Unlock()
	var free, inuse uintptr
	freeBlocks := 0
	for i := range h.nspans {
		sp := &h.spans[i]
		prevFree := false
		b := sp.lo
		for b < sp.hi {
			size := blockSize(b)
			if size < minBlock || size%unit != 0 || b+size > sp.hi {
				return errBlockSize
			}
			if word(b) != word(b+size-wordSize) {
				return errBlockTags
			}
			if isAllocated(b) {
				inuse += size
				prevFree = false
			} else {
				if prevFree {
					return errAdjacent
				}
				free += size
				freeBlocks++
				prevFree = true
			}
			b += size
		}
		if b != sp.hi || word(sp.hi) != allocated {
			return errEpilogue
		}
	}
	listed := 0
	prev := uintptr(0)
	for f := h.freeList; f != 0; f = nextFree(f) {
		if isAllocated(f) || h.spanOf(f) == nil || prevFreeOf(f) != prev {
			return errFreeList
		}
		listed++
		if listed > freeBlocks {
			return errFreeList
		}
		prev = f
	}
	if listed != freeBlocks {
		return errFreeList
	}
	if free != h.stats.Free || inuse != h.stats.InUse {
		return errCounters
	}
	return nil
}

func (h *Heap) alloc(size, align uintptr, zero bool) unsafe.Pointer {
	if align&(align-1) != 0 {
		h.stats.Failures++
		return nil
	}
	align = max(align, DefaultAlign)
	need, ok := blockFor(size)
	if !ok {
		h.stats.Failures++
		return nil
	}

	for b := h.freeList; b != 0; b = nextFree(b) {
		have := blockSize(b)
		payload := b + wordSize
		aligned := alignUp(payload, align)
		gap := aligned - payload
		if gap != 0 && gap < minBlock {
			// The gap is too small to be a free block. Move to the next
			// aligned address that leaves room for one.
			aligned = alignUp(payload+minBlock, align)
			gap = aligned - payload
		}
		if gap+need > have {
			continue
		}

		h.removeFree(b)
		if gap != 0 {
			h.setTags(b, gap, false)
			h.pushFree(b)
			b += gap
			have -= gap
		}
		h.stats.Free -= have
		h.stats.InUse += have
		h.setTags(b, have, true)
		h.shrink(b, need)

		h.stats.Allocs++
		h.notePeak()
		p := b + wordSize
		if zero {
			// Zero the full payload, not only size bytes. Realloc needs
			// zero slack to grow in place.
			clearBytes(p, blockSize(b)-2*wordSize)
		}
		return unsafe.Pointer(p)
	}
	h.stats.Failures++
	return nil
}

func (h *Heap) free(p uintptr) {
	b := p - wordSize
	h.checkAllocated(b)
	size := blockSize(b)
	if h.poison {
		fillBytes(p, size-2*wordSize, poisonByte)
	}
	h.stats.InUse -= size
	h.stats.Free += size
	h.stats.Frees++

	if next := b + size; !isAllocated(next) {
		h.removeFree(next)
		size += blockSize(next)
	}
	if prevFooter := word(b - wordSize); prevFooter&allocated == 0 {
		prev := b - (prevFooter &^ allocated)
		h.removeFree(prev)
		size += b - prev
		b = prev
	}
	h.setTags(b, size, false)
	h.pushFree(b)
}

// shrink cuts the allocated block at b to need bytes. The rest goes to the
// free list if it is large enough to be a block.
func (h *Heap) shrink(b, need uintptr) {
	have := blockSize(b)
	rest := have - need
	if rest < minBlock {
		return
	}
	h.setTags(b, need, true)
	r := b + need
	h.stats.InUse -= rest
	h.stats.Free += rest
	if next := r + rest; !isAllocated(next) {
		h.removeFree(next)
		rest += blockSize(next)
	}
	h.setTags(r, rest, false)
	h.pushFree(r)
}

func (h *Heap) checkAllocated(b uintptr) {
	sp := h.spanOf(b)
	if sp == nil || (b+wordSize)%unit != 0 {
		panic(errInvalidFree)
	}
	if !isAllocated(b) {
		panic(errDoubleFree)
	}
	if size := blockSize(b); size < minBlock || b+size > sp.hi || word(b) != word(b+size-wordSize) {
		panic(errInvalidFree)
	}
}

func (h *Heap) notePeak() {
	if h.stats.InUse > h.stats.Peak {
		h.stats.Peak = h.stats.InUse
	}
}

func (h *Heap) setTags(b, size uintptr, used bool) {
	tag := size
	if used {
		tag |= allocated
	}
	setWord(b, tag)
	setWord(b+size-wordSize, tag)
}

// A block is a header, a payload and a footer. Each tag holds the size and a used bit.
// A free block keeps its next link at b+W and its prev link at b+2W.

func (h *Heap) pushFree(b uintptr) {
	setWord(b+wordSize, h.freeList)
	setWord(b+2*wordSize, 0)
	if h.freeList != 0 {
		setWord(h.freeList+2*wordSize, b)
	}
	h.freeList = b
}

func (h *Heap) removeFree(b uintptr) {
	next, prev := nextFree(b), prevFreeOf(b)
	if prev != 0 {
		setWord(prev+wordSize, next)
	} else {
		h.freeList = next
	}
	if next != 0 {
		setWord(next+2*wordSize, prev)
	}
}

func nextFree(b uintptr) uintptr   { return word(b + wordSize) }
func prevFreeOf(b uintptr) uintptr { return word(b + 2*wordSize) }
func blockSize(b uintptr) uintptr  { return word(b) &^ allocated }
func isAllocated(b uintptr) bool   { return word(b)&allocated != 0 }

// blockFor returns the block size needed for a payload of size bytes.
func blockFor(size uintptr) (uintptr, bool) {
	if size > ^uintptr(0)-2*wordSize-unit {
		return 0, false
	}
	need := alignUp(size+2*wordSize, unit)
	if need < minBlock {
		need = minBlock
	}
	return need, true
}

type noLock struct{}

func (noLock) Lock()   {}
func (noLock) Unlock() {}
