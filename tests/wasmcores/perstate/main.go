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

//go:extern runtime.stackChainStart
//go:threadlocal
var stackChainStart unsafe.Pointer

//go:extern internal/task.currentTask
//go:threadlocal
var currentTask unsafe.Pointer

func where() [2]uintptr {
	return [2]uintptr{uintptr(unsafe.Pointer(&stackChainStart)), uintptr(unsafe.Pointer(&currentTask))}
}

var onCore1 [2]uintptr

//export tinygo_core_entry
func coreEntry(core uint32) {
	onCore1 = where()
}

func main() {
	startCore(1, block(64<<10), block(256)-256)
	time.Sleep(500 * time.Millisecond)
	mine := where()
	for i, name := range []string{"stackChainStart", "currentTask"} {
		if mine[i] == onCore1[i] {
			println("shared", name)
		} else {
			println("per core", name)
		}
	}
}
