//go:build tinygo

package heap

import (
	"internal/gclayout"
	"unsafe"
)

// runtimeAlloc is the TinyGo allocator.
//
//go:linkname runtimeAlloc runtime.alloc
func runtimeAlloc(size uintptr, layout unsafe.Pointer) unsafe.Pointer

// gcAlloc gets GC heap memory that every collector scans conservatively.
// A nil layout is not enough: the Boehm collector reads the layout
// (src/runtime/gc_boehm.go).
func gcAlloc(size uintptr) unsafe.Pointer {
	return runtimeAlloc(size, gclayout.Conservative.AsPtr())
}
