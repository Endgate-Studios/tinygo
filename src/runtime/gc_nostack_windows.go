//go:build gc.leaking || gc.none

package runtime

import _ "unsafe"

// tinygo_scanstack is called by tinygo_scanCurrentStack in asm_*_windows.S,
// which every Windows program links for tinygo_longjmp. These collectors never
// scan the stack, but COFF's linker reports an undefined symbol even in code
// it then drops (ELF's doesn't), so it needs a definition. As for UEFI, see
// gc_leaking_uefi.go.
//
//go:export tinygo_scanstack
func tinygo_scanstack() {
}
