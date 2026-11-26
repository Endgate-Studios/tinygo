package main

import (
	"sync/atomic"
	"time"
	"unsafe"
)

//go:wasmimport tinygo_cores start_core
func startCore(core uint32, stackTop, tls uintptr)

var tlsBlocks [][]byte

// newTLS is a thread-local block for a core (wasm-ld's __tls_size is small).
func newTLS() uintptr {
	b := make([]byte, 256)
	tlsBlocks = append(tlsBlocks, b)
	return (uintptr(unsafe.Pointer(&b[0])) + 15) &^ 15
}

var stacks [][]byte

var fromCore1 atomic.Uint32

//export tinygo_core_entry
func coreEntry(core uint32) {
	println("hello from core", core)
	fromCore1.Store(42)
}

func main() {
	s := make([]byte, 64<<10)
	stacks = append(stacks, s)
	top := (uintptr(unsafe.Pointer(&s[0])) + uintptr(len(s))) &^ 15
	startCore(1, top, newTLS())
	time.Sleep(500 * time.Millisecond)
	println("hello from core 0, core 1 wrote", fromCore1.Load())
}
