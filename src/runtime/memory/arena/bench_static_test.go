//go:build !baremetal

package arena

import "testing"

// benchArena is outside the GC heap and has no pointer slots, so the GC does not read it.
// It is too large for the internal RAM of a microcontroller.
var benchArena [arenaBytes]byte

func BenchmarkGCWithStaticArena(b *testing.B) {
	benchArenaList(b, benchArena[:])
}
