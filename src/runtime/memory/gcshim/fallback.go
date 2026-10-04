//go:build !gc.custom

package gcshim

// Installed reports whether gcshim is this program's garbage collector.
func Installed() bool { return false }

// Reserve returns a normal heap buffer when gcshim is not the collector. -gc=precise does not scan
// it (pointerFree in src/runtime/gc_precise.go), but -gc=conservative scans it on each cycle.
func Reserve(n uintptr) []byte { return make([]byte, n) }

// Release does nothing without gcshim. The GC reclaims the buffer.
func Release(mem []byte) {}

// ReadStats returns zero values without gcshim. Use runtime.ReadMemStats.
func ReadStats() Stats { return Stats{} }
