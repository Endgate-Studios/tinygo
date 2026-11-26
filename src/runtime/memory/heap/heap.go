// Package heap is the memory strategy for the GC heap. Free gives objects back early through
// an injected memory.Freer. A missed Free is safe, because the GC reclaims the memory.
package heap

import (
	"runtime"
	"unsafe"

	"runtime/memory"
	"runtime/memory/free"
)

// maxNativeAlign is the heap alignment of all TinyGo targets (func align in src/runtime/arch_xtensa.go).
// A larger alignment over-allocates. Free then does nothing and the GC reclaims the object.
const maxNativeAlign = 8

// Heap allocates from the GC heap.
type Heap struct {
	freer     memory.Freer
	collector memory.Collector
}

// Option configures a Heap.
type Option func(*options) error

type options struct {
	freer     memory.Freer
	collector memory.Collector
}

// WithFreer sets the Freer that Free uses. The default is free.Runtime{}, which
// releases memory only under -gc=custom with gcshim.
func WithFreer(f memory.Freer) Option {
	return func(o *options) error {
		if f == nil {
			return ErrNilFreer
		}
		o.freer = f
		return nil
	}
}

// WithCollector sets what Collect runs. The default calls runtime.GC.
func WithCollector(c memory.Collector) Option {
	return func(o *options) error {
		if c == nil {
			return ErrNilCollector
		}
		o.collector = c
		return nil
	}
}

// New makes a GC heap strategy.
func New(opts ...Option) (*Heap, error) {
	o := options{freer: free.Runtime{}, collector: runtimeCollector{}}
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return nil, err
		}
	}
	return &Heap{freer: o.freer, collector: o.collector}, nil
}

// Alloc returns size zeroed bytes from the GC heap. The GC scans it, so it can hold GC pointers.
// It returns nil only for a bad align. The runtime panics when memory is full.
func (h *Heap) Alloc(size, align uintptr) unsafe.Pointer {
	if align&(align-1) != 0 {
		return nil
	}
	if align <= maxNativeAlign {
		return gcAlloc(size)
	}
	p := gcAlloc(size + align - 1)
	return unsafe.Pointer((uintptr(p) + align - 1) &^ (align - 1))
}

// Free releases p early through the injected Freer. Without gcshim, the default Freer
// does nothing and the GC reclaims p at its next cycle.
func (h *Heap) Free(p unsafe.Pointer) {
	if p != nil {
		h.freer.Free(p)
	}
}

// Collect runs a collection through the injected Collector.
func (h *Heap) Collect() {
	h.collector.Collect()
}

type runtimeCollector struct{}

func (runtimeCollector) Collect() { runtime.GC() }
