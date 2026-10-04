//go:build gc.custom && baremetal && (esp32 || esp32s3 || esp32c3 || scheduler.cores)

package gcshim

import (
	"runtime/interrupt"
	"unsafe"
)

const (
	pageSize    = 1024
	worklistLen = 128
)

// The heap bounds come from the linker symbols that the built-in collectors use
// (src/runtime/baremetal_memory.go).
//
//go:extern _heap_start
var heapStartSymbol [0]byte

//go:extern _heap_end
var heapEndSymbol [0]byte

func checkAllocContext() {
	if interrupt.In() {
		runtimePanic("heap alloc in interrupt")
	}
}

func initArena() error {
	start := uintptr(unsafe.Pointer(&heapStartSymbol))
	end := uintptr(unsafe.Pointer(&heapEndSymbol))
	return initHeapSpans(start, end)
}

// growArena always fails, because the linker script sets a fixed heap (targets/esp32s3.ld).
func growArena(min uintptr) bool {
	return false
}
