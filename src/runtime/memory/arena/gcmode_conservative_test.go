//go:build gc.conservative

package arena

// The conservative GC reads every word of a heap object (pointerFree in src/runtime/gc_conservative.go).
const (
	gcKnown      = true
	gcReadsBytes = true
)
