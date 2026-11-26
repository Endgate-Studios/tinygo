package main

import "sync"

var sink [16]uint64

func work(slot int) {
	x := uint64(slot) + 1
	for i := 0; i < 3_000_000; i++ {
		x = x*6364136223846793005 + 1442695040888963407
	}
	sink[slot] = x
}

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) { work(i); wg.Done() }(i)
	}
	wg.Wait()
	println("ok")
}
