//go:build gc.custom && (gcshim_precise_globals || windows)

package main

// With gcshim_precise_globals, and always on Windows, gcshim reads only the pointer
// slots of the globals (gcshim/globals_precise.go).
const (
	gcKnown         = true
	heapBytesRead   = true
	globalBytesRead = false
)
