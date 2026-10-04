// Package free releases GC heap objects early through runtime.free. The build selects
// the behavior. README.md (Free shim) has the table and the rules.
package free

import (
	"runtime"
	"unsafe"
)

// Free releases the object that p points to.
func Free[T any](p *T) {
	if p == nil {
		return
	}
	runtimeFree(unsafe.Pointer(p))
}

// FreeSlice releases the backing array of s.
func FreeSlice[T any](s []T) {
	if cap(s) == 0 {
		return
	}
	runtimeFree(unsafe.Pointer(unsafe.SliceData(s)))
}

// IsReal reports whether this build's Free returns memory immediately.
func IsReal() bool {
	return realFree
}

// verifySink makes the Verify probe escape to the heap.
var verifySink []byte

// Verify frees a new heap object and reports whether runtime.MemStats.Frees increased.
// Call it at startup in debug builds to find memory_realfree on a runtime without the patch.
func Verify() bool {
	if !realFree {
		return false
	}
	var before, after runtime.MemStats
	verifySink = make([]byte, 64)
	runtime.ReadMemStats(&before)
	FreeSlice(verifySink)
	verifySink = nil
	runtime.ReadMemStats(&after)
	return after.Frees > before.Frees
}

// Runtime is a memory.Freer that uses runtime.free. Use it where a strategy must
// release GC heap memory early.
type Runtime struct{}

// Free releases p through runtime.free.
func (Runtime) Free(p unsafe.Pointer) {
	if p == nil {
		return
	}
	runtimeFree(p)
}

// IsReal reports whether Free returns memory immediately.
func (Runtime) IsReal() bool {
	return realFree
}
