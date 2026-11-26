package main

/*
_Thread_local unsigned int core_id = 7;
unsigned int get_core(void) { return core_id; }
void set_core(unsigned int c) { core_id = c; }
*/
import "C"

import (
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

//export tinygo_core_entry
func coreEntry(core uint32) {
	println("core", core, "sees", uint32(C.get_core()))
	C.set_core(C.uint(core))
	println("core", core, "set, sees", uint32(C.get_core()))
}

func main() {
	C.set_core(0)
	s := make([]byte, 64<<10)
	stacks = append(stacks, s)
	startCore(1, (uintptr(unsafe.Pointer(&s[0]))+uintptr(len(s)))&^15, newTLS())
	time.Sleep(500 * time.Millisecond)
	println("core 0 sees", uint32(C.get_core()))
}
