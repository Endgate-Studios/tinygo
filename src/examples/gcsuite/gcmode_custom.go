//go:build gc.custom && !gcshim_precise_globals && !windows

package main

// The gcshim collector reads every word of a heap object and of the globals range
// (alloc in gcshim/shim.go, gcshim/globals_range.go).
const (
	gcKnown         = true
	heapBytesRead   = true
	globalBytesRead = true
)
