package memory

import "unsafe"

// New allocates a zeroed T from a. It returns nil when a has no memory.
func New[T any](a Allocator) *T {
	var zero T
	return (*T)(a.Alloc(unsafe.Sizeof(zero), unsafe.Alignof(zero)))
}

// MakeSlice allocates a zeroed []T with len and cap n from a. It returns nil
// if n is negative, if the size overflows, or if a has no memory.
func MakeSlice[T any](a Allocator, n int) []T {
	if n < 0 {
		return nil
	}
	if n == 0 {
		return []T{}
	}
	var zero T
	elem := unsafe.Sizeof(zero)
	if elem != 0 && uintptr(n) > ^uintptr(0)/elem {
		return nil
	}
	p := a.Alloc(elem*uintptr(n), unsafe.Alignof(zero))
	if p == nil {
		return nil
	}
	return unsafe.Slice((*T)(p), n)
}

// Free releases p to f. p must come from New on the same strategy.
func Free[T any](f Freer, p *T) {
	if p == nil {
		return
	}
	f.Free(unsafe.Pointer(p))
}

// FreeSlice releases the backing array of s to f. s must come from MakeSlice.
// A reslice is permitted if it starts at element 0.
func FreeSlice[T any](f Freer, s []T) {
	if cap(s) == 0 {
		return
	}
	f.Free(unsafe.Pointer(unsafe.SliceData(s)))
}
