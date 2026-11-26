//go:build tinygo.wasm64

package runtime

import "unsafe"

// wasm64 links no wasi-libc (its musl is wasm32 only): no C stdio to flush,
// and randomness straight from the host.
func __stdio_exit() {}

//go:wasmimport wasi_snapshot_preview1 random_get
func random_get(buf unsafe.Pointer, bufLen uint) (errno uint16)

func hardwareRand() (n uint64, ok bool) {
	return n, random_get(unsafe.Pointer(&n), 8) == 0
}

// pointerSize tells the JS glue (wasm_exec.js) pointers are 8 bytes.
//
//export tinygo_pointer_size
func pointerSize() uintptr {
	return unsafe.Sizeof(uintptr(0))
}
