//go:build baremetal && !numa_psram_octal

package collector

// The test heaps come from the board heap, which is about 390 KiB on the ESP32-S3.
func init() { maxTestHeap = 128 << 10 }
