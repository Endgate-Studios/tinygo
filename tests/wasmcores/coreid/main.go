package main

import (
	"sync"
	_ "unsafe"
)

//go:linkname currentCPU runtime.currentCPU
func currentCPU() uint32

var mu sync.Mutex
var bad, seen uint32

func probe(g int, wg *sync.WaitGroup) {
	defer wg.Done()
	first := currentCPU()
	for i := 0; i < 2000; i++ {
		mu.Lock()
		c := currentCPU()
		seen++
		if c != first {
			bad++
			if bad < 5 {
				println("goroutine", g, "started on", first, "now on", c)
			}
		}
		mu.Unlock()
	}
}

func main() {
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go probe(g, &wg)
	}
	wg.Wait()
	println("seen", seen, "bad", bad)
}
