package main

import "sync"

type node struct {
	next  *node
	value int
}

var sink [8]*node

func work(g int) {
	for r := 0; r < 20000; r++ {
		sink[g] = &node{next: sink[g], value: r}
		if r%100 == 0 {
			sink[g] = nil
		}
	}
}

func main() {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			work(g)
		}(g)
	}
	wg.Wait()
	println("allocdefer ok")
}
