//go:build !esp32s3

package dsp

func dotInt16(a, b []int16) int64 {
	return dotInt16Go(a, b)
}

func dotInt8(a, b []int8) int64 {
	return dotInt8Go(a, b)
}

func addInt16(dst, a, b []int16) {
	addInt16Go(dst, a, b)
}

func firInt16(dst, x, h []int16, shift uint) {
	firInt16Go(dst, x, h, shift)
}

func matMulInt16(dst, a, b []int16, m, n, k int, shift uint) {
	matMulInt16Go(dst, a, b, m, n, k, shift)
}

func fftInt16(x, w []int16) {
	fftInt16Go(x, w)
}

func mulInt16(dst, a, b []int16, shift uint) {
	mulInt16Go(dst, a, b, shift)
}

func subInt16(dst, a, b []int16)           { subInt16Go(dst, a, b) }
func addInt8(dst, a, b []int8)             { addInt8Go(dst, a, b) }
func subInt8(dst, a, b []int8)             { subInt8Go(dst, a, b) }
func mulInt8(dst, a, b []int8, shift uint) { mulInt8Go(dst, a, b, shift) }
func dotUint16(a, b []uint16) uint64       { return dotUint16Go(a, b) }
func dotUint8(a, b []uint8) uint64         { return dotUint8Go(a, b) }
func copyBytes(dst, src []byte)            { copyBytesGo(dst, src) }
func fillBytes(dst []byte, v byte)         { fillBytesGo(dst, v) }

func firDecimInt16(dst, x, h []int16, decim int, shift uint) {
	firDecimInt16Go(dst, x, h, decim, shift)
}

func dotFloat32(a, b []float32) float32 { return dotFloat32Go(a, b) }

func fftFloat32(f *FFTFloat32, x []float32) { fftFloat32Go(f, x) }

func biquadFloat32(f *BiquadFloat32, dst, src []float32) { biquadFloat32Go(f, dst, src) }

func firFilterFloat32(f *FIRFilterFloat32, dst, src []float32) { firFilterFloat32Go(f, dst, src) }

func matMulFloat32(dst, a, b []float32, m, n, k, aStride, bStride, dstStride int) {
	matMulFloat32Go(dst, a, b, m, n, k, aStride, bStride, dstStride)
}
