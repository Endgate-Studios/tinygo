// Package memory defines the interfaces that every memory strategy implements.
// It also has typed helpers. See README.md for the strategies and safety rules.
package memory

import "unsafe"

// Allocator gives zeroed memory, or nil when it cannot satisfy the request.
// align must be a power of two. Zero selects the default alignment.
type Allocator interface {
	Alloc(size, align uintptr) unsafe.Pointer
}

// Freer releases memory from the Alloc of the same strategy. Free(nil) does nothing.
// A strategy that reclaims memory in a different way can ignore Free.
type Freer interface {
	Free(p unsafe.Pointer)
}

// Collector runs a garbage-collection cycle.
type Collector interface {
	Collect()
}

// Resetter releases all memory from a strategy in one step.
type Resetter interface {
	Reset()
}
