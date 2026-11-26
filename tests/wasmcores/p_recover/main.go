package main

import (
	"sync"
	"time"
)

//go:noinline
func ident(x int) int { return x }

var mu sync.Mutex

// echo returns values through a call in a function with a defer, while
// other cores do the same and the lock makes them pause in between.
func echo(g int, wg *sync.WaitGroup, bad *int) {
	defer wg.Done()
	for i := 0; i < 3000; i++ {
		want := g*100000 + i
		mu.Lock()
		got := ident(want)
		mu.Unlock()
		if got != want {
			mu.Lock()
			*bad++
			mu.Unlock()
		}
	}
}

// recovers panics and recovers on its own core, many times.
func recovers(g int, wg *sync.WaitGroup, caught *int) {
	defer wg.Done()
	for i := 0; i < 200; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					*caught++
					mu.Unlock()
				}
			}()
			if i%2 == 0 {
				time.Sleep(time.Microsecond)
			}
			panic(g)
		}()
	}
}

func main() {
	var wg sync.WaitGroup
	bad, caught := 0, 0
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go recovers(g, &wg, &caught)
	}
	wg.Wait()
	println("wrong results", bad, "recovered", caught, "of", 4*200)
}
