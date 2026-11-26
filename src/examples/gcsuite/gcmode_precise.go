//go:build gc.precise

package main

// The precise GC skips objects without pointers and reads only the pointer slots of the
// globals (src/runtime/gc_precise.go, src/runtime/gc_globals_blocks.go).
const (
	gcKnown         = true
	heapBytesRead   = false
	globalBytesRead = false
)
