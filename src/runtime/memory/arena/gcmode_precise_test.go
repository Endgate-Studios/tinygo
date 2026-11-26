//go:build gc.precise

package arena

// The precise GC skips the words of an object without pointers, such as a []byte
// (pointerFree in src/runtime/gc_precise.go).
const (
	gcKnown      = true
	gcReadsBytes = false
)
