package dsp

import (
	"unsafe"
)

// FIRInt16 filters x with the coefficients h. It sets dst[i] to the sum of
// x[i+k]*h[k] shifted right by shift and saturated to the int16 range. For a
// convolution, give h in reverse order. len(dst) must be len(x)-len(h)+1.
// On the ESP32-S3 it is fastest when len(h) is a multiple of 8.
func FIRInt16(dst, x, h []int16, shift uint) {
	if len(h) == 0 || len(x) < len(h) || len(dst) != len(x)-len(h)+1 {
		panic("dsp: length mismatch")
	}
	if shift > 63 {
		panic("dsp: shift out of range")
	}
	firInt16(dst, x, h, shift)
}

func firInt16Go(dst, x, h []int16, shift uint) {
	for i := range dst {
		v := dotInt16(x[i:i+len(h)], h) >> shift
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		dst[i] = int16(v)
	}
}

// FIRFilterFloat32 is a streaming FIR filter with a delay line, as esp-dsp
// dsps_fir_f32 and dsps_fird_f32. Each output is the sum of coeffs[k] times
// the delay line from the oldest to the newest sample.
type FIRFilterFloat32 struct {
	// state has the layout of the esp-dsp fir_f32_t, which the kernels read.
	state struct {
		coeffs unsafe.Pointer
		delay  unsafe.Pointer
		n      int32
		pos    int32
		decim  int32
	}
	coeffs []float32
	delay  []float32
}

// NewFIRFilterFloat32 returns a filter with a copy of coeffs that keeps every
// decim output. Use decim 1 for no decimation.
func NewFIRFilterFloat32(coeffs []float32, decim int) *FIRFilterFloat32 {
	if len(coeffs) == 0 || decim < 1 {
		panic("dsp: bad filter parameter")
	}
	f := &FIRFilterFloat32{coeffs: alignedFloat32(len(coeffs)), delay: alignedFloat32(len(coeffs) + 4)}
	copy(f.coeffs, coeffs)
	f.state.coeffs = unsafe.Pointer(&f.coeffs[0])
	f.state.delay = unsafe.Pointer(&f.delay[0])
	f.state.n = int32(len(coeffs))
	f.state.decim = int32(decim)
	return f
}

// Process filters src and writes len(src)/decim outputs to dst. It returns the
// number of outputs. len(src) must be a multiple of decim.
func (f *FIRFilterFloat32) Process(dst, src []float32) int {
	d := int(f.state.decim)
	if len(src)%d != 0 || len(dst) < len(src)/d {
		panic("dsp: length mismatch")
	}
	n := len(src) / d
	if n > 0 {
		firFilterFloat32(f, dst[:n], src)
	}
	return n
}

// Reset clears the delay line.
func (f *FIRFilterFloat32) Reset() {
	for i := range f.delay {
		f.delay[i] = 0
	}
	f.state.pos = 0
}

// firFilterFloat32Go is esp-dsp dsps_fird_f32_ansi.
func firFilterFloat32Go(f *FIRFilterFloat32, dst, src []float32) {
	n, d := int(f.state.n), int(f.state.decim)
	pos := int(f.state.pos)
	for i := range dst {
		for k := 0; k < d; k++ {
			f.delay[pos] = src[i*d+k]
			pos++
			if pos >= n {
				pos = 0
			}
		}
		var acc float32
		c := 0
		for j := pos; j < n; j++ {
			acc += f.coeffs[c] * f.delay[j]
			c++
		}
		for j := 0; j < pos; j++ {
			acc += f.coeffs[c] * f.delay[j]
			c++
		}
		dst[i] = acc
	}
	f.state.pos = int32(pos)
}

// FIRFloat32 sets dst[i] to the sum of x[i+k]*h[k]. For a convolution, give h
// in reverse order. len(dst) must be len(x)-len(h)+1.
func FIRFloat32(dst, x, h []float32) {
	FIRDecimFloat32(dst, x, h, 1)
}

// FIRDecimFloat32 is FIRFloat32 that keeps every decim output. len(dst) must be
// (len(x)-len(h))/decim+1.
func FIRDecimFloat32(dst, x, h []float32, decim int) {
	if decim < 1 || len(h) == 0 || len(x) < len(h) || len(dst) != (len(x)-len(h))/decim+1 {
		panic("dsp: length mismatch")
	}
	for i := range dst {
		dst[i] = dotFloat32(x[i*decim:i*decim+len(h)], h)
	}
}

// FIRDecimFilterInt16 is a streaming decimating FIR filter for int16 values,
// as esp-dsp dsps_fird_s16. Each output is the sum of coeffs[k] times the
// input k samples back, with the esp-dsp rounding and shift.
type FIRDecimFilterInt16 struct {
	rev   []int16 // coefficients in reverse order
	delay []int16
	pos   int
	decim int
	dPos  int
	shift int
}

// NewFIRDecimFilterInt16 returns a filter that keeps every decim output.
// startPos is the number of input values to skip before the first output, and
// the result is shifted right by 15-shift, as in esp-dsp dsps_fird_init_s16.
func NewFIRDecimFilterInt16(coeffs []int16, decim, startPos, shift int) *FIRDecimFilterInt16 {
	if len(coeffs) < 2 || decim < 1 || startPos < 0 || startPos >= decim || shift < -40 || shift > 40 {
		panic("dsp: bad filter parameter")
	}
	f := &FIRDecimFilterInt16{rev: AlignedInt16(len(coeffs)), delay: AlignedInt16(len(coeffs)), decim: decim, dPos: startPos, shift: shift}
	for i, c := range coeffs {
		f.rev[len(coeffs)-1-i] = c
	}
	return f
}

// Process filters src and returns the number of outputs written to dst. The
// first output uses decim-startPos inputs and each next output decim inputs,
// so len(src)+startPos must be a multiple of decim. startPos is 0 after the
// first call.
func (f *FIRDecimFilterInt16) Process(dst, src []int16) int {
	if (len(src)+f.dPos)%f.decim != 0 || len(dst) < (len(src)+f.dPos)/f.decim {
		panic("dsp: length mismatch")
	}
	n := len(f.delay)
	rounding := int64(0x7fff)
	if f.shift >= 0 {
		rounding >>= uint(f.shift)
	} else {
		rounding <<= uint(-f.shift)
	}
	rounding &= 0xFFFFFFFFFF
	finalShift := f.shift - 15
	in, out := 0, 0
	for in < len(src) {
		for j := f.dPos; j < f.decim; j++ {
			if f.pos >= n {
				f.pos = 0
			}
			f.delay[f.pos] = src[in]
			f.pos++
			in++
		}
		f.dPos = 0
		acc := rounding + dotInt16(f.delay[f.pos:], f.rev[:n-f.pos]) + dotInt16(f.delay[:f.pos], f.rev[n-f.pos:])
		if finalShift > 0 {
			dst[out] = int16(acc << uint(finalShift))
		} else {
			dst[out] = int16(acc >> uint(-finalShift))
		}
		out++
	}
	return out
}

// FIRMultiRateFloat32 is a streaming FIR filter that interpolates by interp
// and decimates by decim, as esp-dsp dsps_firmr_f32.
type FIRMultiRateFloat32 struct {
	coeffs   []float32
	delay    []float32
	pos      int
	interp   int
	Decim    int
	startPos int
}

// NewFIRMultiRateFloat32 returns the filter. len(coeffs) must be a multiple of
// interp, and startPos must be below decim.
func NewFIRMultiRateFloat32(coeffs []float32, interp, decim, startPos int) *FIRMultiRateFloat32 {
	if interp < 1 || decim < 1 || len(coeffs)%interp != 0 || startPos < 0 || startPos >= decim {
		panic("dsp: bad filter parameter")
	}
	return &FIRMultiRateFloat32{coeffs: coeffs, delay: make([]float32, len(coeffs)/interp), interp: interp, Decim: decim, startPos: startPos}
}

// Process filters src and returns the number of outputs written to dst. dst
// needs space for len(src)*interp/decim+1 values.
func (f *FIRMultiRateFloat32) Process(dst, src []float32) int {
	size := len(f.delay)
	out := 0
	for _, v := range src {
		f.delay[f.pos] = v
		m := f.startPos
		for ; m < f.interp; m += f.Decim {
			var sum float32
			c := 0
			for n := f.pos; n < size; n++ {
				sum += f.delay[n] * f.coeffs[c*f.interp+m]
				c++
			}
			for n := 0; n < f.pos; n++ {
				sum += f.delay[n] * f.coeffs[c*f.interp+m]
				c++
			}
			dst[out] = sum
			out++
		}
		f.startPos = m - f.interp
		f.pos--
		if f.pos < 0 {
			f.pos = size - 1
		}
	}
	return out
}

// FIRMultiRateInt16 is FIRMultiRateFloat32 for int16 values, as esp-dsp
// dsps_firmr_s16. The result is shifted right by 15-shift.
type FIRMultiRateInt16 struct {
	coeffs   []int16
	delay    []int16
	pos      int
	interp   int
	Decim    int
	startPos int
	shift    int
}

// NewFIRMultiRateInt16 returns the filter.
func NewFIRMultiRateInt16(coeffs []int16, interp, decim, startPos, shift int) *FIRMultiRateInt16 {
	if interp < 1 || decim < 1 || len(coeffs)%interp != 0 || startPos < 0 || startPos >= decim || shift < -40 || shift > 40 {
		panic("dsp: bad filter parameter")
	}
	return &FIRMultiRateInt16{coeffs: coeffs, delay: make([]int16, len(coeffs)/interp), interp: interp, Decim: decim, startPos: startPos, shift: shift}
}

// Process filters src and returns the number of outputs written to dst.
func (f *FIRMultiRateInt16) Process(dst, src []int16) int {
	size := len(f.delay)
	rounding := int64(0x7fff)
	if f.shift >= 0 {
		rounding >>= uint(f.shift)
	} else {
		rounding <<= uint(-f.shift)
	}
	rounding &= 0xFFFFFFFFFF
	finalShift := f.shift - 15
	out := 0
	for _, v := range src {
		f.delay[f.pos] = v
		m := f.startPos
		for ; m < f.interp; m += f.Decim {
			acc := rounding
			c := 0
			for n := f.pos; n < size; n++ {
				acc += int64(f.delay[n]) * int64(f.coeffs[c*f.interp+m])
				c++
			}
			for n := 0; n < f.pos; n++ {
				acc += int64(f.delay[n]) * int64(f.coeffs[c*f.interp+m])
				c++
			}
			if finalShift > 0 {
				dst[out] = int16(acc << uint(finalShift))
			} else {
				dst[out] = int16(acc >> uint(-finalShift))
			}
			out++
		}
		f.startPos = m - f.interp
		f.pos--
		if f.pos < 0 {
			f.pos = size - 1
		}
	}
	return out
}

// FIRDecimInt16 filters x with h and keeps every decim output. It sets dst[i]
// to the sum of x[i*decim+k]*h[k], shifted right by shift and saturated.
// len(dst) must be (len(x)-len(h))/decim+1.
func FIRDecimInt16(dst, x, h []int16, decim int, shift uint) {
	if decim < 1 || len(h) == 0 || len(x) < len(h) || len(dst) != (len(x)-len(h))/decim+1 {
		panic("dsp: length mismatch")
	}
	if shift > 63 {
		panic("dsp: shift out of range")
	}
	firDecimInt16(dst, x, h, decim, shift)
}

func firDecimInt16Go(dst, x, h []int16, decim int, shift uint) {
	for i := range dst {
		p := x[i*decim : i*decim+len(h)]
		dst[i] = sat16(dotInt16(p, h) >> shift)
	}
}
