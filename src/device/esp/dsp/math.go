package dsp

import (
	"math"
)

// AddInt16 sets dst[i] to a[i]+b[i], saturated to the int16 range. It panics
// if the lengths differ.
func AddInt16(dst, a, b []int16) {
	if len(a) != len(b) || len(dst) != len(a) {
		panic("dsp: length mismatch")
	}
	addInt16(dst, a, b)
}

// MulInt16 sets dst[i] to int16(int32(a[i])*int32(b[i]) >> shift). The result
// is truncated to 16 bits, as the PIE EE.VMUL.S16 instruction does. It panics
// if the lengths differ or if shift is more than 31.
func MulInt16(dst, a, b []int16, shift uint) {
	if len(a) != len(b) || len(dst) != len(a) {
		panic("dsp: length mismatch")
	}
	if shift > 31 {
		panic("dsp: shift out of range")
	}
	mulInt16(dst, a, b, shift)
}

func mulInt16Go(dst, a, b []int16, shift uint) {
	for i := range a {
		dst[i] = int16(int32(a[i]) * int32(b[i]) >> shift)
	}
}

func addInt16Go(dst, a, b []int16) {
	for i := range a {
		v := int32(a[i]) + int32(b[i])
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		dst[i] = int16(v)
	}
}

// MulConstInt16 sets dst[i] to int16(int32(src[i])*int32(c) >> 15), as
// esp-dsp dsps_mulc_s16.
func MulConstInt16(dst, src []int16, c int16) {
	checkLen2(len(dst), len(src))
	for i, v := range src {
		dst[i] = int16(int32(v) * int32(c) >> 15)
	}
}

// ReverseInt16 reverses the order of s, as esp-dsp dsps_16_array_rev.
func ReverseInt16(s []int16) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// SqrtApprox returns an approximate square root with the bit method of esp-dsp
// dsps_sqrtf_f32. It is faster than math.Sqrt, with an error of a few percent.
func SqrtApprox(x float32) float32 {
	return math.Float32frombits(0x1fbb4000 + math.Float32bits(x)>>1)
}

// InvSqrtApprox returns an approximate 1/sqrt(x), as esp-dsp
// dsps_inverted_sqrtf_f32. It uses one Newton step.
func InvSqrtApprox(x float32) float32 {
	y := math.Float32frombits(0x5f3759df - math.Float32bits(x)>>1)
	return y * (1.5 - x*0.5*y*y)
}

// AddFloat32 sets dst[i] to a[i]+b[i].
func AddFloat32(dst, a, b []float32) {
	checkLen3(len(dst), len(a), len(b))
	for i := range a {
		dst[i] = a[i] + b[i]
	}
}

// SubFloat32 sets dst[i] to a[i]-b[i].
func SubFloat32(dst, a, b []float32) {
	checkLen3(len(dst), len(a), len(b))
	for i := range a {
		dst[i] = a[i] - b[i]
	}
}

// MulFloat32 sets dst[i] to a[i]*b[i].
func MulFloat32(dst, a, b []float32) {
	checkLen3(len(dst), len(a), len(b))
	for i := range a {
		dst[i] = a[i] * b[i]
	}
}

// AddConstFloat32 sets dst[i] to src[i]+c.
func AddConstFloat32(dst, src []float32, c float32) {
	checkLen2(len(dst), len(src))
	for i := range src {
		dst[i] = src[i] + c
	}
}

// MulConstFloat32 sets dst[i] to src[i]*c.
func MulConstFloat32(dst, src []float32, c float32) {
	checkLen2(len(dst), len(src))
	for i := range src {
		dst[i] = src[i] * c
	}
}

// SqrtFloat32 sets dst[i] to the square root of src[i].
func SqrtFloat32(dst, src []float32) {
	checkLen2(len(dst), len(src))
	for i := range src {
		dst[i] = float32(math.Sqrt(float64(src[i])))
	}
}

// SubInt16 sets dst[i] to a[i]-b[i], saturated to the int16 range.
func SubInt16(dst, a, b []int16) {
	checkLen3(len(dst), len(a), len(b))
	subInt16(dst, a, b)
}

// AddInt8 sets dst[i] to a[i]+b[i], saturated to the int8 range.
func AddInt8(dst, a, b []int8) {
	checkLen3(len(dst), len(a), len(b))
	addInt8(dst, a, b)
}

// SubInt8 sets dst[i] to a[i]-b[i], saturated to the int8 range.
func SubInt8(dst, a, b []int8) {
	checkLen3(len(dst), len(a), len(b))
	subInt8(dst, a, b)
}

// MulInt8 sets dst[i] to int8(int16(a[i])*int16(b[i]) >> shift). The result is
// truncated to 8 bits, as the PIE EE.VMUL.S8 instruction does.
func MulInt8(dst, a, b []int8, shift uint) {
	checkLen3(len(dst), len(a), len(b))
	if shift > 15 {
		panic("dsp: shift out of range")
	}
	mulInt8(dst, a, b, shift)
}

// Copy copies src to dst like the built in copy, and returns the number of
// bytes. Aligned buffers use 16 byte PIE loads and stores on the ESP32-S3.
func Copy(dst, src []byte) int {
	n := min(len(dst), len(src))
	copyBytes(dst[:n], src[:n])
	return n
}

// Fill sets every byte of dst to v.
func Fill(dst []byte, v byte) {
	fillBytes(dst, v)
}

func subInt16Go(dst, a, b []int16) {
	for i := range a {
		dst[i] = sat16(int64(a[i]) - int64(b[i]))
	}
}

func addInt8Go(dst, a, b []int8) {
	for i := range a {
		dst[i] = sat8(int32(a[i]) + int32(b[i]))
	}
}

func subInt8Go(dst, a, b []int8) {
	for i := range a {
		dst[i] = sat8(int32(a[i]) - int32(b[i]))
	}
}

func mulInt8Go(dst, a, b []int8, shift uint) {
	for i := range a {
		dst[i] = int8(int16(a[i]) * int16(b[i]) >> shift)
	}
}

func copyBytesGo(dst, src []byte) {
	copy(dst, src)
}

func fillBytesGo(dst []byte, v byte) {
	for i := range dst {
		dst[i] = v
	}
}

// The Ex functions are the esp-dsp math functions with all parameters. They
// work on n values at a step of stepA, stepB and stepDst. The integer versions
// shift the result right by shift and truncate it to the type, as the esp-dsp
// ansi code does. AddInt16 and the other short versions saturate instead.

func AddInt16Ex(dst, a, b []int16, n, stepA, stepB, stepDst int, shift uint) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int16((int32(a[i*stepA]) + int32(b[i*stepB])) >> shift)
	}
}

func SubInt16Ex(dst, a, b []int16, n, stepA, stepB, stepDst int, shift uint) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int16((int32(a[i*stepA]) - int32(b[i*stepB])) >> shift)
	}
}

// MulInt16Ex uses the PIE code of MulInt16 when all steps are 1.
func MulInt16Ex(dst, a, b []int16, n, stepA, stepB, stepDst int, shift uint) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	if stepA == 1 && stepB == 1 && stepDst == 1 && shift <= 31 {
		mulInt16(dst[:n], a[:n], b[:n], shift)
		return
	}
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int16(int32(a[i*stepA]) * int32(b[i*stepB]) >> shift)
	}
}

func AddInt8Ex(dst, a, b []int8, n, stepA, stepB, stepDst int, shift uint) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int8((int32(a[i*stepA]) + int32(b[i*stepB])) >> shift)
	}
}

func SubInt8Ex(dst, a, b []int8, n, stepA, stepB, stepDst int, shift uint) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int8((int32(a[i*stepA]) - int32(b[i*stepB])) >> shift)
	}
}

// MulInt8Ex uses the PIE code of MulInt8 when all steps are 1.
func MulInt8Ex(dst, a, b []int8, n, stepA, stepB, stepDst int, shift uint) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	if stepA == 1 && stepB == 1 && stepDst == 1 && shift <= 15 {
		mulInt8(dst[:n], a[:n], b[:n], shift)
		return
	}
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int8(int32(a[i*stepA]) * int32(b[i*stepB]) >> shift)
	}
}

// MulConstInt16Ex is MulConstInt16 with steps, as esp-dsp dsps_mulc_s16.
func MulConstInt16Ex(dst, src []int16, n int, c int16, stepSrc, stepDst int) {
	checkSteps(n, len(dst), stepDst, len(src), stepSrc, len(src), stepSrc)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = int16(int32(src[i*stepSrc]) * int32(c) >> 15)
	}
}

func AddFloat32Ex(dst, a, b []float32, n, stepA, stepB, stepDst int) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = a[i*stepA] + b[i*stepB]
	}
}

func SubFloat32Ex(dst, a, b []float32, n, stepA, stepB, stepDst int) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = a[i*stepA] - b[i*stepB]
	}
}

func MulFloat32Ex(dst, a, b []float32, n, stepA, stepB, stepDst int) {
	checkSteps(n, len(dst), stepDst, len(a), stepA, len(b), stepB)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = a[i*stepA] * b[i*stepB]
	}
}

func AddConstFloat32Ex(dst, src []float32, n int, c float32, stepSrc, stepDst int) {
	checkSteps(n, len(dst), stepDst, len(src), stepSrc, len(src), stepSrc)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = src[i*stepSrc] + c
	}
}

func MulConstFloat32Ex(dst, src []float32, n int, c float32, stepSrc, stepDst int) {
	checkSteps(n, len(dst), stepDst, len(src), stepSrc, len(src), stepSrc)
	for i := 0; i < n; i++ {
		dst[i*stepDst] = src[i*stepSrc] * c
	}
}

// SqrtApproxFloat32 sets dst[i] to SqrtApprox(src[i]), as esp-dsp dsps_sqrt_f32.
func SqrtApproxFloat32(dst, src []float32) {
	checkLen2(len(dst), len(src))
	for i, v := range src {
		dst[i] = SqrtApprox(v)
	}
}
