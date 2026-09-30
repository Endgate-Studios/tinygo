//go:build esp32s3

package dsp

import "unsafe"

// The kernels are in src/device/esp/esp32s3_dsp.S, see targets/esp32s3.json.

//export tinygo_dsp_dot_s16
func asmDotS16(a, b *int16, n int) int64

//export tinygo_dsp_dot_s16_unaligned
func asmDotS16Unaligned(a, b *int16, n int) int64

//export tinygo_dsp_dot_s8
func asmDotS8(a, b *int8, n int) int64

//export tinygo_dsp_dot_s8_unaligned
func asmDotS8Unaligned(a, b *int8, n int) int64

//export tinygo_dsp_dot_u16
func asmDotU16(a, b *uint16, n int) uint64

//export tinygo_dsp_dot_u8
func asmDotU8(a, b *uint8, n int) uint64

//export tinygo_dsp_add_s16
func asmAddS16(dst, a, b *int16, n int)

//export tinygo_dsp_sub_s16
func asmSubS16(dst, a, b *int16, n int)

//export tinygo_dsp_add_s8
func asmAddS8(dst, a, b *int8, n int)

//export tinygo_dsp_sub_s8
func asmSubS8(dst, a, b *int8, n int)

//export tinygo_dsp_mul_s16
func asmMulS16(dst, a, b *int16, n int, shift uint)

//export tinygo_dsp_mul_s8
func asmMulS8(dst, a, b *int8, n int, shift uint)

//export tinygo_dsp_copy
func asmCopy(dst, src *byte, n int)

//export tinygo_dsp_fill
func asmFill(dst, value *byte, n int)

//export tinygo_dsp_matmul_s16_block
func asmMatMulS16Block(dst, a, b *int16, n, kBytes int, shift uint)

//export tinygo_dsp_fft2r_sc16
func asmFFT2rS16(data *int16, n int, w *int16)

//export tinygo_dsp_dotprod_f32
func asmDotF32(a, b, dest *float32, n int)

//export tinygo_dsp_fft2r_fc32
func asmFFT2rF32(data *float32, n int, w *float32)

//export tinygo_dsp_fft4r_fc32
func asmFFT4rF32(data *float32, n int, table *float32, tableSize int)

//export tinygo_dsp_bit_rev_lookup_fc32
func asmBitRevLookupF32(data *float32, count int, table *uint16)

//export tinygo_dsp_fir_f32
func asmFIRF32(fir unsafe.Pointer, input, output *float32, n int)

//export tinygo_dsp_fird_f32
func asmFIRDF32(fir unsafe.Pointer, input, output *float32, n int) int32

//export tinygo_dsp_biquad_f32
func asmBiquadF32(input, output *float32, n int, coef, w *float32)

//export tinygo_dsp_mult_f32
func asmMultF32(a, b, c *float32, m, n, k int)

//export tinygo_dsp_mult_ex_f32
func asmMultExF32(a, b, c *float32, m, n, k, aPad, bPad, cPad int)

func aligned(p unsafe.Pointer) bool {
	return uintptr(p)%16 == 0
}

func alignedAll(ptrs ...unsafe.Pointer) bool {
	for _, p := range ptrs {
		if !aligned(p) {
			return false
		}
	}
	return true
}

// ACCX holds 40 bits. The chunks keep the sum below 2^39.
const (
	chunkS16 = 256  // 256 products of 2^30
	chunkS8  = 4096 // 4096 products of 2^14
	chunkU16 = 64   // 64 products of 2^32
	chunkU8  = 4096 // 4096 products of 2^16
)

func dotInt16(a, b []int16) int64 {
	n := len(a) &^ 7
	if n == 0 {
		return dotInt16Go(a, b)
	}
	if !aligned(unsafe.Pointer(&b[0])) {
		a, b = b, a
	}
	if !aligned(unsafe.Pointer(&b[0])) {
		return dotInt16Go(a, b)
	}
	unaligned := !aligned(unsafe.Pointer(&a[0]))
	var sum int64
	for i := 0; i < n; i += chunkS16 {
		c := min(chunkS16, n-i)
		if unaligned {
			sum += asmDotS16Unaligned(&a[i], &b[i], c)
		} else {
			sum += asmDotS16(&a[i], &b[i], c)
		}
	}
	return sum + dotInt16Go(a[n:], b[n:])
}

func dotInt8(a, b []int8) int64 {
	n := len(a) &^ 15
	if n == 0 {
		return dotInt8Go(a, b)
	}
	if !aligned(unsafe.Pointer(&b[0])) {
		a, b = b, a
	}
	if !aligned(unsafe.Pointer(&b[0])) {
		return dotInt8Go(a, b)
	}
	unaligned := !aligned(unsafe.Pointer(&a[0]))
	var sum int64
	for i := 0; i < n; i += chunkS8 {
		c := min(chunkS8, n-i)
		if unaligned {
			sum += asmDotS8Unaligned(&a[i], &b[i], c)
		} else {
			sum += asmDotS8(&a[i], &b[i], c)
		}
	}
	return sum + dotInt8Go(a[n:], b[n:])
}

func dotUint16(a, b []uint16) uint64 {
	n := len(a) &^ 7
	if n == 0 || !alignedAll(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0])) {
		return dotUint16Go(a, b)
	}
	var sum uint64
	for i := 0; i < n; i += chunkU16 {
		sum += asmDotU16(&a[i], &b[i], min(chunkU16, n-i))
	}
	return sum + dotUint16Go(a[n:], b[n:])
}

func dotUint8(a, b []uint8) uint64 {
	n := len(a) &^ 15
	if n == 0 || !alignedAll(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0])) {
		return dotUint8Go(a, b)
	}
	var sum uint64
	for i := 0; i < n; i += chunkU8 {
		sum += asmDotU8(&a[i], &b[i], min(chunkU8, n-i))
	}
	return sum + dotUint8Go(a[n:], b[n:])
}

// The element wise kernels.
const (
	opAdd = iota
	opSub
	opMul
)

// binaryInt16 runs the kernel for op on the aligned part and Go on the rest.
func binaryInt16(op int, dst, a, b []int16, shift uint) {
	n := len(a) &^ 7
	if n > 0 && alignedAll(unsafe.Pointer(&dst[0]), unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0])) {
		switch op {
		case opAdd:
			asmAddS16(&dst[0], &a[0], &b[0], n)
		case opSub:
			asmSubS16(&dst[0], &a[0], &b[0], n)
		default:
			asmMulS16(&dst[0], &a[0], &b[0], n, shift)
		}
		dst, a, b = dst[n:], a[n:], b[n:]
	}
	switch op {
	case opAdd:
		addInt16Go(dst, a, b)
	case opSub:
		subInt16Go(dst, a, b)
	default:
		mulInt16Go(dst, a, b, shift)
	}
}

func binaryInt8(op int, dst, a, b []int8, shift uint) {
	n := len(a) &^ 15
	if n > 0 && alignedAll(unsafe.Pointer(&dst[0]), unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0])) {
		switch op {
		case opAdd:
			asmAddS8(&dst[0], &a[0], &b[0], n)
		case opSub:
			asmSubS8(&dst[0], &a[0], &b[0], n)
		default:
			asmMulS8(&dst[0], &a[0], &b[0], n, shift)
		}
		dst, a, b = dst[n:], a[n:], b[n:]
	}
	switch op {
	case opAdd:
		addInt8Go(dst, a, b)
	case opSub:
		subInt8Go(dst, a, b)
	default:
		mulInt8Go(dst, a, b, shift)
	}
}

func addInt16(dst, a, b []int16)             { binaryInt16(opAdd, dst, a, b, 0) }
func subInt16(dst, a, b []int16)             { binaryInt16(opSub, dst, a, b, 0) }
func mulInt16(dst, a, b []int16, shift uint) { binaryInt16(opMul, dst, a, b, shift) }
func addInt8(dst, a, b []int8)               { binaryInt8(opAdd, dst, a, b, 0) }
func subInt8(dst, a, b []int8)               { binaryInt8(opSub, dst, a, b, 0) }
func mulInt8(dst, a, b []int8, shift uint)   { binaryInt8(opMul, dst, a, b, shift) }

func firInt16(dst, x, h []int16, shift uint) {
	firDecimInt16(dst, x, h, 1, shift)
}

// firDecimInt16 calls the dot kernels directly for each output when one call
// covers all taps.
func firDecimInt16(dst, x, h []int16, decim int, shift uint) {
	taps := len(h)
	if taps%8 != 0 || taps > chunkS16 || !aligned(unsafe.Pointer(&h[0])) {
		firDecimInt16Go(dst, x, h, decim, shift)
		return
	}
	for i := range dst {
		p := &x[i*decim]
		var v int64
		if aligned(unsafe.Pointer(p)) {
			v = asmDotS16(p, &h[0], taps)
		} else {
			v = asmDotS16Unaligned(p, &h[0], taps)
		}
		dst[i] = sat16(v >> shift)
	}
}

func matMulInt16(dst, a, b []int16, m, n, k int, shift uint) {
	// QACC holds 40 bits per lane, and 256 products of 2^30 fit in 2^38.
	if m == 0 || k%8 != 0 || n == 0 || n > 256 || shift > 31 || !aligned(unsafe.Pointer(&b[0])) || !aligned(unsafe.Pointer(&dst[0])) {
		matMulInt16Go(dst, a, b, m, n, k, shift)
		return
	}
	for i := 0; i < m; i++ {
		for j := 0; j < k; j += 8 {
			asmMatMulS16Block(&dst[i*k+j], &a[i*n], &b[j], n, 2*k, shift)
		}
	}
}

func fftInt16(x, w []int16) {
	n := len(x) / 2
	if n < 8 || !alignedAll(unsafe.Pointer(&x[0]), unsafe.Pointer(&w[0])) {
		fftInt16Go(x, w)
		return
	}
	asmFFT2rS16(&x[0], n, &w[0])
}

func copyBytes(dst, src []byte) {
	n := len(dst) &^ 15
	if n == 0 || !alignedAll(unsafe.Pointer(&dst[0]), unsafe.Pointer(&src[0])) {
		copyBytesGo(dst, src)
		return
	}
	asmCopy(&dst[0], &src[0], n)
	copyBytesGo(dst[n:], src[n:])
}

func fillBytes(dst []byte, v byte) {
	n := len(dst) &^ 15
	if n == 0 || !aligned(unsafe.Pointer(&dst[0])) {
		fillBytesGo(dst, v)
		return
	}
	asmFill(&dst[0], &v, n)
	fillBytesGo(dst[n:], v)
}

func dotFloat32(a, b []float32) float32 {
	n := len(a) &^ 3
	if n == 0 || !alignedAll(unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0])) {
		return dotFloat32Go(a, b)
	}
	var r float32
	asmDotF32(&a[0], &b[0], &r, n)
	return r + dotFloat32Go(a[n:], b[n:])
}

func fftFloat32(f *FFTFloat32, x []float32) {
	n := len(x) / 2
	if uintptr(unsafe.Pointer(&x[0]))%8 != 0 {
		fftFloat32Go(f, x)
		return
	}
	if bitsOf(n)%2 == 0 {
		asmFFT4rF32(&x[0], n, &f.w4[0], f.n)
		if n == f.n && len(f.rev4) > 0 {
			asmBitRevLookupF32(&x[0], len(f.rev4)/2, &f.rev4[0])
		} else {
			digitReverse4Float32(x)
		}
		return
	}
	asmFFT2rF32(&x[0], n, &f.w2[0])
	if n == f.n && len(f.rev2) > 0 {
		asmBitRevLookupF32(&x[0], len(f.rev2)/2, &f.rev2[0])
	} else {
		bitReverseFloat32(x)
	}
}

// digitReverse4Float32 puts the complex values of x in base 4 digit reversed order.
func digitReverse4Float32(x []float32) {
	n := len(x) / 2
	digits := bitsOf(n) / 2
	for i := 0; i < n; i++ {
		if j := reverseDigits4(i, digits); i < j {
			x[2*i], x[2*j] = x[2*j], x[2*i]
			x[2*i+1], x[2*j+1] = x[2*j+1], x[2*i+1]
		}
	}
}

func biquadFloat32(f *BiquadFloat32, dst, src []float32) {
	// The kernel reads one input past the last one, so the last sample is done in Go.
	if n := len(src) - 1; n > 0 {
		asmBiquadF32(&src[0], &dst[0], n, &f.B0, &f.w1)
		dst, src = dst[n:], src[n:]
	}
	biquadFloat32Go(f, dst, src)
}

func firFilterFloat32(f *FIRFilterFloat32, dst, src []float32) {
	if f.state.n%4 != 0 {
		firFilterFloat32Go(f, dst, src)
		return
	}
	if f.state.decim == 1 {
		asmFIRF32(unsafe.Pointer(&f.state), &src[0], &dst[0], len(dst))
	} else {
		asmFIRDF32(unsafe.Pointer(&f.state), &src[0], &dst[0], len(dst))
	}
}

func matMulFloat32(dst, a, b []float32, m, n, k, aStride, bStride, dstStride int) {
	aPad, bPad, dPad := aStride-n, bStride-k, dstStride-k
	if m == 0 || n == 0 || k == 0 || (m|n|k|aPad|bPad|dPad)%4 != 0 ||
		!alignedAll(unsafe.Pointer(&dst[0]), unsafe.Pointer(&a[0]), unsafe.Pointer(&b[0])) {
		matMulFloat32Go(dst, a, b, m, n, k, aStride, bStride, dstStride)
		return
	}
	if aPad == 0 && bPad == 0 && dPad == 0 {
		asmMultF32(&a[0], &b[0], &dst[0], m, n, k)
		return
	}
	asmMultExF32(&a[0], &b[0], &dst[0], m, n, k, aPad, bPad, dPad)
}
