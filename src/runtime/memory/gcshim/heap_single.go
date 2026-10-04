//go:build gc.custom && !(gcshim_psram && baremetal && esp32s3 && numa_psram_octal)

package gcshim

import "runtime/memory/collector"

// gcHeap is one collector over one span.
type gcHeap = collector.Collector

// heapSpans is empty, because shim.heap holds the only collector.
type heapSpans struct{}
