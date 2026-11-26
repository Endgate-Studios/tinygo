package main

import (
	"time"
	_ "unsafe"
)

//go:linkname currentCPU runtime.currentCPU
func currentCPU() uint32

func main() {
	done := make(chan bool)
	for g := 0; g < 3; g++ {
		go func(g int) {
			for i := 0; i < 5; i++ {
				c := currentCPU()
				if c >= 8 {
					println("goroutine", g, "BAD core", c)
				}
				time.Sleep(time.Millisecond)
			}
			done <- true
		}(g)
	}
	for g := 0; g < 3; g++ {
		<-done
	}
	println("migrate ok")
}
