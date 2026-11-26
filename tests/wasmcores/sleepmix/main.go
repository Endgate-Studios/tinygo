package main

import (
	"sync"
	"time"
)

var mu sync.Mutex
var n int

func main() {
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 3000; i++ {
				mu.Lock()
				n++
				mu.Unlock()
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				time.Sleep(time.Microsecond)
			}
		}()
	}
	wg.Wait()
	println("sleepmix ok", n)
}
