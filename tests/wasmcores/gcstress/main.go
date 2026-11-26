package main

import (
	"runtime"
	"sync"
)

type node struct {
	next  *node
	value int
	pad   [6]int
}

// build is a list of n nodes holding seed, seed+1, ...
func build(seed, n int) *node {
	var head *node
	for i := n - 1; i >= 0; i-- {
		head = &node{next: head, value: seed + i}
	}
	return head
}

// sum checks a list: its values in order.
func sum(head *node, seed, n int) bool {
	i := 0
	for p := head; p != nil; p = p.next {
		if p.value != seed+i {
			return false
		}
		i++
	}
	return i == n
}

func main() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	bad := 0
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			// Kept lists, checked as others come and go.
			kept := make([]*node, 0, 8)
			for round := 0; round < 400; round++ {
				seed := g*1_000_000 + round*1000
				l := build(seed, 500)
				if round%50 == 0 {
					kept = append(kept, l)
				}
				if !sum(l, seed, 500) {
					mu.Lock()
					bad++
					mu.Unlock()
				}
				for k, kl := range kept {
					if !sum(kl, g*1_000_000+k*50*1000, 500) {
						mu.Lock()
						bad++
						mu.Unlock()
					}
				}
			}
		}(g)
	}
	wg.Wait()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	println("gcstress bad", bad, "collections", ms.NumGC, "allocated MB", ms.TotalAlloc>>20, "heap MB", ms.HeapSys>>20)
}
