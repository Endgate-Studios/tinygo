package main

import (
	"sync"
	"sync/atomic"
	"time"
)

var failed bool

func check(name string, got, want int) {
	if got != want {
		failed = true
		println("FAIL", name, "got", got, "want", want)
	}
}

func main() {
	const workers = 8

	// Mutex: a counter many goroutines add to.
	{
		var mu sync.Mutex
		var wg sync.WaitGroup
		n := 0
		for g := 0; g < workers; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 5000; i++ {
					mu.Lock()
					n++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		check("mutex", n, workers*5000)
	}

	// RWMutex: writers add, readers see a consistent pair.
	{
		var rw sync.RWMutex
		var wg sync.WaitGroup
		a, b, torn := 0, 0, int32(0)
		for g := 0; g < workers; g++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				for i := 0; i < 2000; i++ {
					rw.Lock()
					a++
					b++
					rw.Unlock()
				}
			}()
			go func() {
				defer wg.Done()
				for i := 0; i < 2000; i++ {
					rw.RLock()
					if a != b {
						atomic.AddInt32(&torn, 1)
					}
					rw.RUnlock()
				}
			}()
		}
		wg.Wait()
		check("rwmutex writes", a, workers*2000)
		check("rwmutex torn reads", int(torn), 0)
	}

	// Unbuffered and buffered channels: every value through once.
	for _, size := range []int{0, 16} {
		ch := make(chan int, size)
		var wg sync.WaitGroup
		var sum int64
		for g := 0; g < workers; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < 2000; i++ {
					ch <- 1
				}
			}(g)
		}
		done := make(chan bool)
		go func() {
			for v := range ch {
				sum += int64(v)
			}
			done <- true
		}()
		wg.Wait()
		close(ch)
		<-done
		check("channel", int(sum), workers*2000)
	}

	// Select across channels with a timeout that never fires.
	{
		a, b := make(chan int), make(chan int)
		var wg sync.WaitGroup
		for g := 0; g < workers; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for i := 0; i < 1000; i++ {
					if i%2 == 0 {
						a <- 1
					} else {
						b <- 1
					}
				}
			}(g)
		}
		got, timeouts := 0, 0
		for got < workers*1000 {
			select {
			case v := <-a:
				got += v
			case v := <-b:
				got += v
			case <-time.After(5 * time.Second):
				timeouts++
				got = workers * 1000
			}
		}
		wg.Wait()
		check("select timeouts", timeouts, 0)
	}

	// Cond: a value handed through, one at a time.
	{
		var mu sync.Mutex
		cond := sync.NewCond(&mu)
		ready, taken := false, 0
		var wg sync.WaitGroup
		for g := 0; g < workers; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 200; i++ {
					mu.Lock()
					for !ready {
						cond.Wait()
					}
					ready = false
					taken++
					cond.Broadcast()
					mu.Unlock()
				}
			}()
		}
		for i := 0; i < workers*200; i++ {
			mu.Lock()
			for ready {
				cond.Wait()
			}
			ready = true
			cond.Broadcast()
			mu.Unlock()
		}
		wg.Wait()
		check("cond", taken, workers*200)
	}

	// Once, atomics and many short goroutines.
	{
		var once sync.Once
		var calls, adds int64
		var wg sync.WaitGroup
		for g := 0; g < 2000; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				once.Do(func() { atomic.AddInt64(&calls, 1) })
				atomic.AddInt64(&adds, 1)
			}()
		}
		wg.Wait()
		check("once", int(calls), 1)
		check("short goroutines", int(adds), 2000)
	}

	// Timers on many goroutines at once.
	{
		var wg sync.WaitGroup
		var woke int64
		for g := 0; g < 64; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				time.Sleep(time.Duration(g%8) * time.Millisecond)
				atomic.AddInt64(&woke, 1)
			}(g)
		}
		wg.Wait()
		check("timers", int(woke), 64)
	}

	if !failed {
		println("stress ok")
	}
}
