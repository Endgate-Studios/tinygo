package arena

import "unsafe"

func word(addr uintptr) uintptr {
	return *(*uintptr)(unsafe.Pointer(addr))
}

func setWord(addr, v uintptr) {
	*(*uintptr)(unsafe.Pointer(addr)) = v
}

func bytesAt(addr, n uintptr) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(addr)), n)
}

func clearBytes(addr, n uintptr) {
	if n != 0 {
		clear(bytesAt(addr, n))
	}
}

func fillBytes(addr, n uintptr, b byte) {
	if n == 0 {
		return
	}
	s := bytesAt(addr, n)
	for i := range s {
		s[i] = b
	}
}

func copyBytes(dst, src, n uintptr) {
	if n != 0 {
		copy(bytesAt(dst, n), bytesAt(src, n))
	}
}

func alignUp(v, a uintptr) uintptr {
	return (v + a - 1) &^ (a - 1)
}

func alignDown(v, a uintptr) uintptr {
	return v &^ (a - 1)
}
