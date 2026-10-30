//go:build tinygo

package free

import "unsafe"

// runtimeFree is runtime.free. Under -gc=custom, gcshim supplies it (src/runtime/gc_custom.go).
// The built-in collectors free only manual allocations (src/runtime/gc_blocks.go).
//
//go:linkname runtimeFree runtime.free
func runtimeFree(ptr unsafe.Pointer)
