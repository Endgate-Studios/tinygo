package main

import (
	"time"
	"unsafe"
)

//go:wasmimport tinygo_cores start_core
func startCore(core uint32, stackTop, tls uintptr)

var keep [][]byte

func block(n int) uintptr {
	b := make([]byte, n)
	keep = append(keep, b)
	return (uintptr(unsafe.Pointer(&b[0])) + uintptr(n)) &^ 15
}

//go:threadlocal
var mine uint32 = 7

var shared uint32

//export tinygo_core_entry
func coreEntry(core uint32) {
	println("core", core, "mine", mine)
	mine = core
	shared = 99
	println("core", core, "set mine", mine)
}

func main() {
	mine = 0
	tls := block(256) - 256
	startCore(1, block(64<<10), tls)
	time.Sleep(500 * time.Millisecond)
	println("core 0 mine", mine, "shared", shared)
}
