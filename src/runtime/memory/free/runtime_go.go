//go:build !tinygo

package free

import "unsafe"

// runtimeFree does nothing on the standard Go toolchain, which has no explicit free.
// It lets packages that use the shim build and test on the host.
func runtimeFree(ptr unsafe.Pointer) {}
