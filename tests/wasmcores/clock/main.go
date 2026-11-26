package main

import "time"

func main() {
	t := time.Now()
	time.Sleep(50 * time.Millisecond)
	println("sleep 50ms measured", time.Since(t).Milliseconds())
	println("loop start")
	t = time.Now()
	x := uint64(1)
	for i := 0; i < 100_000_000; i++ {
		x = x*6364136223846793005 + 1442695040888963407
	}
	println("loop ms", time.Since(t).Milliseconds(), x&1)
}
