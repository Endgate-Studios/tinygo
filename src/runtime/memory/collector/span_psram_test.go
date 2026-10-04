//go:build numa_psram_octal

package collector

import (
	"testing"
	"unsafe"
)

// psramPool holds the test heaps in PSRAM. The .psram section comes from targets/esp32s3.ld,
// and the runtime maps and zeroes it (src/runtime/runtime_esp32_psram.go).
//
//go:section .psram
var psramPool [4 << 20]byte

// psramUsed is the end of the memory that running tests hold, as in a stack.
var psramUsed uintptr

// testSpan returns zeroed memory for one test heap from psramPool, and gives it back when the test ends.
func testSpan(t *testing.T, bytes uintptr) []uintptr {
	start := (psramUsed + 15) &^ 15
	if start+bytes > uintptr(len(psramPool)) {
		t.Skipf("a test heap of %d bytes does not fit in the PSRAM pool", bytes)
	}
	before := psramUsed
	psramUsed = start + bytes
	t.Cleanup(func() { psramUsed = before })
	s := unsafe.Slice((*uintptr)(unsafe.Pointer(&psramPool[start])), bytes/wordSize)
	clear(s)
	return s
}
