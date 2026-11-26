//go:build esp32p4

package machine

import (
	"runtime/volatile"
	"unsafe"
)

// LastCrash is what was printed before the last panic or exception, once, or nothing. Only USB builds
// keep it. The layout is in runtime_esp32p4_usbotg.go.
func LastCrash() string {
	word := func(i uintptr) *volatile.Register32 {
		return (*volatile.Register32)(unsafe.Pointer(uintptr(0x50108000) + i*4))
	}
	const size = 512
	crashed := word(0).Get() == 0x43524153
	next := word(1).Get()
	word(0).Set(0)
	word(1).Set(0)
	if !crashed {
		return ""
	}
	start, n := uint32(0), next
	if next > size {
		start, n = next-size, size
	}
	text := make([]byte, n)
	for i := uint32(0); i < n; i++ {
		text[i] = byte(word(2 + uintptr((start+i)%size)).Get())
	}
	return string(text)
}
