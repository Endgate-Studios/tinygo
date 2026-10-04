//go:build !tinygo

package heap

import "unsafe"

// gcAlloc uses pointer-typed words so that the precise host GC keeps referenced objects alive.
// It is for host tests. Store only real pointers or plain data in it.
func gcAlloc(size uintptr) unsafe.Pointer {
	words := (size + unsafe.Sizeof(uintptr(0)) - 1) / unsafe.Sizeof(uintptr(0))
	if words == 0 {
		words = 1
	}
	return unsafe.Pointer(unsafe.SliceData(make([]unsafe.Pointer, words)))
}
