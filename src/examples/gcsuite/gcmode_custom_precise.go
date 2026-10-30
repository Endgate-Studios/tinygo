//go:build gc.custom && gcshim_precise_globals

package main

// With gcshim_precise_globals, gcshim reads only the pointer slots of the globals
// (gcshim/globals_precise.go).
const (
	gcKnown         = true
	heapBytesRead   = true
	globalBytesRead = false
)
