//go:build gc.custom && baremetal && (esp32 || esp32s3 || esp32c3 || esp32p4 || scheduler.cores) && !(gcshim_psram && esp32s3 && numa_psram_octal)

package gcshim

func initHeapSpans(start, end uintptr) error {
	return shim.heap.Init(start, end, end, collectorConfig())
}
