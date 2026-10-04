package collector

import (
	"testing"
	"unsafe"
)

// BenchmarkCycle measures one full collection of a large pointer array that holds
// 20000 small objects, like test/gcbench.
func BenchmarkCycle(b *testing.B) {
	span := make([]uintptr, (4<<20)/wordSize)
	start := uintptr(unsafe.Pointer(&span[0]))
	var c Collector
	if err := c.Init(start, start+4<<20, start+64<<20/8, Config{PageSize: 4096, WorklistLen: 1024}); err != nil {
		b.Fatal(err)
	}
	const n = 20000
	arr := uintptr(c.Alloc(n * wordSize))
	for i := uintptr(0); i < n; i++ {
		*(*uintptr)(unsafe.Pointer(arr + i*wordSize)) = uintptr(c.Alloc(28))
	}
	roots := []uintptr{arr}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.BeginCycle()
		c.MarkRange(uintptr(unsafe.Pointer(&roots[0])), uintptr(unsafe.Pointer(&roots[0]))+wordSize)
		c.FinishMark()
		c.Sweep()
	}
	_ = span[0]
}

// BenchmarkAllocCollect allocates 28-byte objects with no live data, and collects when the heap
// is full, so it measures allocation with the cost of the cycles spread over it.
func BenchmarkAllocCollect(b *testing.B) {
	for _, heap := range []uintptr{256 << 10, 4 << 20} {
		b.Run(sizeName(heap), func(b *testing.B) {
			span := make([]uintptr, heap/wordSize)
			start := uintptr(unsafe.Pointer(&span[0]))
			var c Collector
			if err := c.Init(start, start+heap, start+heap, Config{PageSize: 4096, WorklistLen: 1024}); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if c.Alloc(28) == nil {
					c.BeginCycle()
					c.FinishMark()
					c.Sweep()
					if c.Alloc(28) == nil {
						b.Fatal("no memory after a full cycle")
					}
				}
			}
			_ = span[0]
		})
	}
}

func sizeName(n uintptr) string {
	if n >= 1<<20 {
		return string(rune('0'+n>>20)) + "MiB"
	}
	return "256KiB"
}
