//go:build gc.conservative

package main

// The conservative GC reads every word of a heap object, but only the pointer slots of the
// globals (src/runtime/gc_conservative.go, src/runtime/gc_globals_blocks.go).
const (
	gcKnown         = true
	heapBytesRead   = true
	globalBytesRead = false
)
