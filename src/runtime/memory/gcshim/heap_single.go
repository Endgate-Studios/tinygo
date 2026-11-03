//go:build gc.custom && !(gcshim_psram && baremetal && esp32s3 && numa_psram_octal)

package gcshim

import (
	"runtime/memory/collector"
	"unsafe"
)

// gcHeap is one collector over one span.
type gcHeap = collector.Collector

// allocFirst is the span's Alloc: there is no other span.
func allocFirst(size uintptr, noPointers bool) unsafe.Pointer { return allocAny(size, noPointers) }

// heapSpans is empty, because shim.heap holds the only collector.
type heapSpans struct{}

// SpanStats returns the one span's counters as the first, and zero for the
// second, which this build does not have.
func SpanStats() (first, second collector.Stats) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return shim.heap.Stats(), collector.Stats{}
}
