//go:build gc.custom && tinygo.wasm && !gcshim_precise_globals

package gcshim

// The globals end where the heap starts (src/runtime/arch_tinygowasm.go).
//
//go:extern __global_base
var globalsStartSymbol [0]byte

//go:extern __heap_base
var globalsEndSymbol [0]byte
