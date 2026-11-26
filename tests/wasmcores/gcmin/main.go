package main

import "runtime"

type node struct {
	next  *node
	value int
}

func main() {
	var keep *node
	for i := 0; i < 100; i++ {
		keep = &node{next: keep, value: i}
	}
	println("before GC")
	runtime.GC()
	println("after GC 1")
	for r := 0; r < 20; r++ {
		var junk *node
		for i := 0; i < 10000; i++ {
			junk = &node{next: junk, value: i}
		}
		_ = junk
		runtime.GC()
	}
	n := 0
	for p := keep; p != nil; p = p.next {
		n++
	}
	println("gcmin kept", n)
}
