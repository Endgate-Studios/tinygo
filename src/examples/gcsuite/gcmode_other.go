//go:build !gc.conservative && !gc.precise && !gc.custom

package main

// The arena GC tests skip the other collectors.
const (
	gcKnown         = false
	heapBytesRead   = false
	globalBytesRead = false
)
