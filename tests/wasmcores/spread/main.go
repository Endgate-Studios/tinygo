package main

import (
	"runtime"
	"sync"
	"time"
	_ "unsafe"
)

//go:linkname currentCPU runtime.currentCPU
func currentCPU() uint32

var sink [16]uint64

// work is CPU-bound: nothing allocated, nothing shared but its own slot.
func work(slot int) uint32 {
	x := uint64(slot) + 1
	for i := 0; i < 30_000_000; i++ {
		x = x*6364136223846793005 + 1442695040888963407
	}
	sink[slot] = x
	return currentCPU()
}

func main() {
	jobs := runtime.NumCPU() - 1
	if jobs < 2 {
		jobs = 2
	}
	t0 := time.Now()
	for i := 0; i < jobs; i++ {
		work(i)
	}
	serial := time.Since(t0)

	var wg sync.WaitGroup
	var mu sync.Mutex
	used := map[uint32]bool{}
	t1 := time.Now()
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := work(i)
			mu.Lock()
			used[c] = true
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	parallel := time.Since(t1)
	var check uint64
	for _, v := range sink {
		check ^= v
	}
	println("check", check&0xffff)
	println("jobs", jobs, "cores used", len(used), "serial ms", serial.Milliseconds(), "parallel ms", parallel.Milliseconds(),
		"speedup x10", int(10*float64(serial)/float64(parallel)))
}
