//go:build !gc.custom

package gcshim

import "runtime/memory/collector"

// Installed reports whether gcshim is this program's garbage collector.
func Installed() bool { return false }

// Reserve returns a normal heap buffer when gcshim is not the collector. -gc=precise does not scan
// it (pointerFree in src/runtime/gc_precise.go), but -gc=conservative scans it on each cycle.
func Reserve(n uintptr) []byte { return make([]byte, n) }

// Release does nothing without gcshim. The GC reclaims the buffer.
func Release(mem []byte) {}

// ReadStats returns zero values without gcshim. Use runtime.ReadMemStats.
func ReadStats() Stats { return Stats{} }

// SpanStats returns zero values without gcshim. Use runtime.ReadMemStats.
func SpanStats() (first, second collector.Stats) { return }

// BytesByCore returns zero values without gcshim.
func BytesByCore() (core0, core1 uint64) { return }

// AllocsByCore returns zero values without gcshim.
func AllocsByCore() (core0, core1 uint64) { return }

// Scope is gcshim's allocation scope; without gcshim, allocations go to the
// heap as usual and Scope does nothing.
type Scope struct{ Overflows uint32 }

// NewScope returns a Scope that does nothing.
func NewScope(mem []byte) *Scope { return &Scope{} }

func (s *Scope) Enter()                     {}
func (s *Scope) Exit()                      {}
func (s *Scope) Reset()                     {}
func (s *Scope) Used() uintptr              { return 0 }
func (s *Scope) Live() (start, end uintptr) { return 0, 0 }

// AddRoot returns ErrNotInstalled without gcshim: the built-in collectors
// have no registered roots. -gc=conservative reads heap and global memory
// anyway, but -gc=precise does not read a byte array.
func AddRoot(r Root) error { return ErrNotInstalled }

// RemoveRoot does nothing without gcshim.
func RemoveRoot(r Root) {}
