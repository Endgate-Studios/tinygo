package arena

import (
	"unsafe"
)

// RegionStats is a snapshot of Region counters.
type RegionStats struct {
	Allocs   uint64
	Resets   uint64
	Failures uint64
	Cap      uintptr // total bytes
	Used     uintptr // bytes given since the last Reset, with padding
	Peak     uintptr // highest Used seen
}

// Region is a bump allocator for scratch memory. Reset releases all allocations at once.
// Region is not safe for concurrent use. Give each goroutine its own Region.
type Region struct {
	stats          RegionStats // first, for 8-byte alignment
	mem            []byte      // keeps the memory alive under a precise GC
	base, off, end uintptr
	poison         bool
}

// RegionOption configures a Region.
type RegionOption func(*regionOptions) error

type regionOptions struct {
	poison bool
}

// WithRegionPoison fills memory with 0xDE on Reset to make use after Reset visible.
func WithRegionPoison(on bool) RegionOption {
	return func(o *regionOptions) error {
		o.poison = on
		return nil
	}
}

// NewRegion makes a Region over mem. mem must stay alive while the Region is in use.
func NewRegion(mem []byte, opts ...RegionOption) (*Region, error) {
	o := regionOptions{poison: debugPoison}
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	if len(mem) == 0 {
		return nil, ErrEmptyRegion
	}
	base := uintptr(unsafe.Pointer(unsafe.SliceData(mem)))
	r := &Region{mem: mem, base: base, off: base, end: base + uintptr(len(mem)), poison: o.poison}
	r.stats.Cap = uintptr(len(mem))
	return r, nil
}

// Live is the part of the region in use, [start, end). With it, a Region is a
// gcshim.Root: gcshim.AddRoot(r) has the collector scan what r holds, so the
// region may hold the only pointer to a heap object. Without that, the
// collector may not read the region's memory at all.
func (r *Region) Live() (start, end uintptr) { return r.base, r.off }

// Alloc returns size zeroed bytes aligned to align, or nil if the region is full.
// An align of 0 selects DefaultAlign.
func (r *Region) Alloc(size, align uintptr) unsafe.Pointer {
	if align == 0 {
		align = DefaultAlign
	}
	if align&(align-1) != 0 {
		r.stats.Failures++
		return nil
	}
	p := alignUp(r.off, align)
	if p < r.off || p > r.end || size > r.end-p {
		r.stats.Failures++
		return nil
	}
	r.off = p + size
	clearBytes(p, size)
	r.stats.Allocs++
	r.stats.Used = r.off - r.base
	if r.stats.Used > r.stats.Peak {
		r.stats.Peak = r.stats.Used
	}
	return unsafe.Pointer(p)
}

// Reset releases all allocations. Do not use pointers into the region after Reset.
func (r *Region) Reset() {
	if r.poison {
		fillBytes(r.base, r.off-r.base, poisonByte)
	}
	r.off = r.base
	r.stats.Used = 0
	r.stats.Resets++
}

// Mark returns the current fill level, for a later Rewind.
func (r *Region) Mark() uintptr {
	return r.off - r.base
}

// Rewind releases all allocations made after Mark returned m. It ignores m
// if m is past the current fill level.
func (r *Region) Rewind(m uintptr) {
	if m > r.off-r.base {
		return
	}
	if r.poison {
		fillBytes(r.base+m, r.off-r.base-m, poisonByte)
	}
	r.off = r.base + m
	r.stats.Used = m
}

// Stats returns a snapshot of the counters.
func (r *Region) Stats() RegionStats {
	return r.stats
}
