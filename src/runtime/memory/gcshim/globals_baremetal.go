//go:build gc.custom && baremetal && !gcshim_precise_globals

package gcshim

// These are the symbols that src/runtime/baremetal_memory.go reads.
//
//go:extern _globals_start
var globalsStartSymbol [0]byte

//go:extern _globals_end
var globalsEndSymbol [0]byte
