//go:build gc.custom && !gcshim_precise_globals

package gcshim

import "unsafe"

// markGlobals scans the full globals range conservatively. TinyGo 0.43 removes
// runtime.findGlobals, so gcshim reads the linker symbols itself (tinygo commit 59fb1040).
func markGlobals() {
	markRoots(uintptr(unsafe.Pointer(&globalsStartSymbol)), uintptr(unsafe.Pointer(&globalsEndSymbol)))
}
