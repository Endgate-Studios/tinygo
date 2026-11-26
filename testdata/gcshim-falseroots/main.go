package main

// Words that only look like pointers don't keep an 8MB buffer alive: stale
// pointers below a live goroutine's stack pointer, and a uintptr global. A
// big buffer covers so many addresses that text and numbers in globals and
// stale stack words often fall inside it.

import (
	"runtime"
	"unsafe"
)

var frameSum uintptr
var address uintptr

//go:noinline
func fillFrame(frame []uintptr) {
	for i := range frame {
		frame[i] = uintptr(i)
		frameSum += frame[i]
	}
}

// newBuffer allocates depth frames down, leaving stale copies of the
// pointer below the caller's stack pointer.
//
//go:noinline
func newBuffer(depth int) *[8 << 20]byte {
	var frame [4]uintptr
	fillFrame(frame[:])
	if depth == 0 {
		return new([8 << 20]byte)
	}
	b := newBuffer(depth - 1)
	frameSum += frame[0]
	return b
}

//go:noinline
func useBuffer() {
	b := newBuffer(16)
	b[0] = 1
	frameSum += uintptr(b[0])
}

//go:noinline
func keepAddress() {
	b := new([8 << 20]byte)
	b[0] = 1
	address = uintptr(unsafe.Pointer(&b[4<<20]))
}

func check(what string) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if ms.HeapAlloc >= 8<<20 {
		println(what+": kept,", ms.HeapAlloc>>20, "MB in use")
	} else {
		println(what + ": freed")
	}
}

func main() {
	ready := make(chan struct{})
	block := make(chan struct{})
	go func() {
		useBuffer()
		ready <- struct{}{}
		<-block
	}()
	<-ready
	check("stale stack word")
	close(block)

	done := make(chan struct{})
	go func() {
		keepAddress()
		close(done)
	}()
	<-done
	check("uintptr global")
}
