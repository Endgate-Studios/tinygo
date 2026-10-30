//go:build gc.custom && baremetal && esp32s3 && numa_psram_octal && gcshim_psram

package gcshim

import "unsafe"

//go:extern _globals_start
var scopeGlobalsStart [0]byte

//go:extern _globals_end
var scopeGlobalsEnd [0]byte

// Holders finds what points into s: words in the globals, the heap (live
// objects and garbage alike, goroutine stacks too) and the static PSRAM
// data (other Scopes among it) whose value is an
// address in s. It reports at most limit of them, the word's address and its
// value, and returns how many it found. It is for finding what keeps a
// pointer into a Scope past its Reset: slow, and for debugging only.
func (s *Scope) Holders(limit int, report func(where, value uintptr)) int {
	shim.lock.Lock()
	lo, hi := s.base, s.top
	shim.lock.Unlock()
	found := 0
	scan := func(start, end uintptr) {
		for a := (start + 3) &^ 3; a+4 <= end; a += 4 {
			if v := *(*uintptr)(unsafe.Pointer(a)); v-lo < hi-lo {
				if found < limit {
					report(a, v)
				}
				found++
			}
		}
	}
	scan(uintptr(unsafe.Pointer(&scopeGlobalsStart)), uintptr(unsafe.Pointer(&scopeGlobalsEnd)))
	scan(shim.spans.dram.Bounds())
	scan(shim.spans.psram.Bounds())
	// The static PSRAM data, other Scopes among it, but not s itself.
	ps, pe := uintptr(unsafe.Pointer(&psramStartSymbol)), uintptr(unsafe.Pointer(&psramDataEndSymbol))
	scan(ps, min(s.base, pe))
	scan(max(s.end, ps), pe)
	return found
}
