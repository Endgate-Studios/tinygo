//go:build tinygo.wasm && !wasm_unknown && !wasip2 && !tinygo.wasm64

package runtime

// Flush stdio on exit.
//
//export __stdio_exit
func __stdio_exit()

func hardwareRand() (n uint64, ok bool) {
	n |= uint64(libc_arc4random())
	n |= uint64(libc_arc4random()) << 32
	return n, true
}

// uint32_t arc4random(void);
//
//export arc4random
func libc_arc4random() uint32
