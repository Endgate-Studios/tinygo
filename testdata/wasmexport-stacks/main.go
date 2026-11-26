package main

import "runtime"

// Each //go:wasmexport call runs in a goroutine of its own. Once the call
// returns, that goroutine is gone: it isn't counted and its stack is freed.

var sink []byte

//go:wasmexport ping
func ping() {
	sink = make([]byte, 64)
}

//go:wasmexport goroutines
func goroutines() int32 {
	return int32(runtime.NumGoroutine())
}

func main() {}
