//go:build !numa_psram_octal

package collector

import "testing"

// testSpan returns zeroed memory for one test heap.
func testSpan(t *testing.T, bytes uintptr) []uintptr {
	return make([]uintptr, bytes/wordSize)
}
