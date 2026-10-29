//go:build gc.custom && !(gcshim_psram && baremetal && esp32s3 && numa_psram_octal)

package gcshim

import (
	"runtime/memory/collector"
	"unsafe"
)

// gcHeap is one collector over one span.
type gcHeap = collector.Collector

// allocFirst is the span's Alloc: there is no other span.
func allocFirst(size uintptr) unsafe.Pointer { return shim.heap.Alloc(size) }

// heapSpans is empty, because shim.heap holds the only collector.
type heapSpans struct{}
