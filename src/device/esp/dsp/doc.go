// Package dsp has the signal processing functions of esp-dsp
// (https://github.com/espressif/esp-dsp) for TinyGo. On the ESP32-S3 the
// integer functions use the PIE SIMD instructions and the float functions use
// the esp-dsp FPU kernels, from src/device/esp/esp32s3_dsp.S. On other targets
// all functions are plain Go with the same results, except where noted.
//
// The files follow the esp-dsp modules: dotprod, math, matrix, fft, dct, fir,
// iir, resampler, conv, image, window, signal, analysis and view.
//
// The fast code needs 16 byte aligned slices. AlignedInt16 and AlignedInt8 make
// such slices, because slices on the stack can be unaligned. For DotInt16,
// DotInt8 and the FIR filters one aligned input is enough. Other slices use
// the plain Go code.
package dsp

import (
	"unsafe"
)

// AlignedInt16 returns a slice of n values that starts at a 16 byte boundary.
func AlignedInt16(n int) []int16 {
	s := make([]int16, n+8)
	off := int(-uintptr(unsafe.Pointer(&s[0])) % 16 / 2)
	return s[off : off+n : off+n]
}

// AlignedInt8 returns a slice of n values that starts at a 16 byte boundary.
func AlignedInt8(n int) []int8 {
	s := make([]int8, n+16)
	off := int(-uintptr(unsafe.Pointer(&s[0])) % 16)
	return s[off : off+n : off+n]
}

// alignedFloat32 returns a slice of n values that starts at a 16 byte boundary.
func alignedFloat32(n int) []float32 {
	s := make([]float32, n+4)
	off := 0
	for uintptrOf(s[off:])%16 != 0 {
		off++
	}
	return s[off : off+n : off+n]
}

func uintptrOf(s []float32) uintptr {
	return uintptr(unsafe.Pointer(&s[0]))
}

func checkRegion(lenA, aStride, lenB, bStride, width, height int) {
	if width < 0 || height < 0 || aStride < width || bStride < width {
		panic("dsp: bad region")
	}
	if height > 0 && ((height-1)*aStride+width > lenA || (height-1)*bStride+width > lenB) {
		panic("dsp: bad region")
	}
}

func checkLen2(a, b int) {
	if a != b {
		panic("dsp: length mismatch")
	}
}

func checkLen3(a, b, c int) {
	if a != b || a != c {
		panic("dsp: length mismatch")
	}
}

func sat16(v int64) int16 {
	if v > 32767 {
		return 32767
	} else if v < -32768 {
		return -32768
	}
	return int16(v)
}

func sat8(v int32) int8 {
	if v > 127 {
		return 127
	} else if v < -128 {
		return -128
	}
	return int8(v)
}

// checkSteps panics if n values at a step do not fit in the slices.
func checkSteps(n, lenD, stepD, lenA, stepA, lenB, stepB int) {
	if n < 0 || stepD < 1 || stepA < 1 || stepB < 1 {
		panic("dsp: bad length or step")
	}
	if n > 0 && ((n-1)*stepD >= lenD || (n-1)*stepA >= lenA || (n-1)*stepB >= lenB) {
		panic("dsp: length mismatch")
	}
}
