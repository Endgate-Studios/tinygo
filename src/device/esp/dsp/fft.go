package dsp

import (
	"math"
)

// The transforms here use f for the FFT, so len(x) is limited by the size of
// f. They are not scaled, as in esp-dsp. They use a buffer in f, so do not use
// one FFTFloat32 from two goroutines at the same time.

// DCT replaces x with its DCT-II, the sum of x[n]*cos(pi*(n+0.5)*k/N), as
// esp-dsp dsps_dct_f32. len(x) must be a power of two and at most the size of f.
func (f *FFTFloat32) DCT(x []float32) {
	n := len(x)
	f.checkTransform(n, n)
	v := f.buffer(2 * n)
	for i := 0; i < n/2; i++ {
		v[2*i], v[2*i+1] = x[2*i], 0
		v[2*(n-1-i)], v[2*(n-1-i)+1] = x[2*i+1], 0
	}
	f.Transform(v)
	tw := f.twiddles(n, 0)
	for k := 0; k < n; k++ {
		x[k] = v[2*k]*tw[2*k] + v[2*k+1]*tw[2*k+1]
	}
}

// IDCT replaces x with its DCT-III, x[0]/2 plus the sum of
// x[k]*cos(pi*k*(n+0.5)/N) for k from 1, as esp-dsp dsps_dct_inv_f32. IDCT of
// DCT returns x times N/2.
func (f *FFTFloat32) IDCT(x []float32) {
	n := len(x)
	f.checkTransform(n, n)
	v := f.buffer(2 * n)
	tw := f.twiddles(n, 0)
	for k := 0; k < n; k++ {
		a := x[k]
		if k == 0 {
			a /= 2
		}
		v[2*k], v[2*k+1] = a*tw[2*k], -a*tw[2*k+1]
	}
	f.Transform(v)
	for m := 0; m < n/2; m++ {
		x[2*m] = v[2*m]
		x[2*m+1] = v[2*(n-1-m)]
	}
}

// DCTIV replaces x with its DCT-IV, the sum of x[n]*cos(pi/N*(n+0.5)*(k+0.5)),
// as esp-dsp dsps_dctiv_f32. len(x) must be a power of two of at least 2 and at
// most twice the size of f.
func (f *FFTFloat32) DCTIV(x []float32) {
	n := len(x)
	f.checkTransform(n, n/2)
	h := n / 2
	z := f.buffer(n)
	pre, post := f.twiddles(n, 1), f.twiddles(n, 2)
	for i := 0; i < h; i++ {
		re, im := x[2*i], x[n-1-2*i]
		c, s := pre[2*i], pre[2*i+1]
		z[2*i] = re*c - im*s
		z[2*i+1] = re*s + im*c
	}
	f.Transform(z)
	for k := 0; k < h; k++ {
		re, im := z[2*k], z[2*k+1]
		c, s := post[2*k], post[2*k+1]
		yr := re*c - im*s
		yi := re*s + im*c
		x[2*k] = yr
		x[n-1-2*k] = -yi
	}
}

// DSTIV replaces x with its DST-IV, the sum of x[n]*sin(pi/N*(n+0.5)*(k+0.5)),
// as esp-dsp dsps_dstiv_f32. The limits are as for DCTIV.
func (f *FFTFloat32) DSTIV(x []float32) {
	n := len(x)
	for i := 1; i < n; i += 2 {
		x[i] = -x[i]
	}
	f.DCTIV(x)
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		x[i], x[j] = x[j], x[i]
	}
}

func (f *FFTFloat32) checkTransform(n, points int) {
	if n < 2 || n&(n-1) != 0 || points > f.n {
		panic("dsp: transform length")
	}
}

// buffer returns a scratch buffer of n values that is kept in f.
func (f *FFTFloat32) buffer(n int) []float32 {
	if len(f.scratch) < n {
		f.scratch = alignedFloat32(n)
	}
	return f.scratch[:n]
}

// twiddles returns cos and sin pairs for the transforms of n values, and keeps
// them in f. kind 0 is pi*k/(2n) for the DCT, kind 1 is -pi*(4k+1)/(4n) and
// kind 2 is -pi*k/n for the DCT-IV.
func (f *FFTFloat32) twiddles(n, kind int) []float32 {
	if t := f.tw[kind]; len(t) == 2*n {
		return t
	}
	count := n
	if kind != 0 {
		count = n / 2
	}
	t := make([]float32, 2*n)
	for k := 0; k < count; k++ {
		var a float64
		switch kind {
		case 0:
			a = math.Pi * float64(k) / float64(2*n)
		case 1:
			a = -math.Pi * float64(4*k+1) / float64(4*n)
		default:
			a = -math.Pi * float64(k) / float64(n)
		}
		c, s := cosSin(a)
		t[2*k], t[2*k+1] = float32(c), float32(s)
	}
	f.tw[kind] = t
	return t
}

// FFTInt16 is a radix-2 FFT for complex int16 values. It uses the method and
// the scaling of esp-dsp dsps_fft2r_sc16.
type FFTInt16 struct {
	w []int16
}

// NewFFTInt16 returns an FFT for up to n complex values. n must be a power of
// two and at least 2.
func NewFFTInt16(n int) *FFTInt16 {
	if n < 2 || n&(n-1) != 0 {
		panic("dsp: FFT size must be a power of two")
	}
	w := AlignedInt16(n)
	e := 2 * math.Pi / float64(n)
	for i := 0; i < n/2; i++ {
		c, s := cosSin(float64(i) * e)
		w[2*i] = int16(math.MaxInt16 * c)
		w[2*i+1] = int16(math.MaxInt16 * s)
	}
	bitReverse(w)
	return &FFTInt16{w: w}
}

// Transform replaces x with its discrete Fourier transform divided by the
// number of values. x holds the complex values as real and imaginary pairs.
// The number of values must be a power of two and at most the size of f.
// On the ESP32-S3 the PIE code rounds less exactly, so the result can differ
// from other targets by a few LSB. It passes the esp-dsp noise test.
func (f *FFTInt16) Transform(x []int16) {
	n := len(x) / 2
	if len(x)%2 != 0 || n < 1 || n&(n-1) != 0 || n > len(f.w) {
		panic("dsp: FFT length")
	}
	fftInt16(x, f.w)
	bitReverse(x)
}

func fftInt16Go(x, w []int16) {
	n := len(x) / 2
	ie := 1
	for n2 := n / 2; n2 > 0; n2 >>= 1 {
		ia := 0
		for j := 0; j < ie; j++ {
			c, s := int32(w[2*j]), int32(w[2*j+1])
			for i := 0; i < n2; i++ {
				m := ia + n2
				mre, mim := int32(x[2*m]), int32(x[2*m+1])
				are, aim := int32(x[2*ia]), int32(x[2*ia+1])
				tre := c*mre + s*mim
				tim := c*mim - s*mre
				x[2*m] = int16((are*0x7fff - tre + 0x7fff) >> 16)
				x[2*m+1] = int16((aim*0x7fff - tim + 0x7fff) >> 16)
				x[2*ia] = int16((are*0x7fff + tre + 0x7fff) >> 16)
				x[2*ia+1] = int16((aim*0x7fff + tim + 0x7fff) >> 16)
				ia++
			}
			ia += n2
		}
		ie <<= 1
	}
}

// cosSin returns the cosine and the sine of a. LLVM can join math.Cos and
// math.Sin into sincos, which some C libraries do not have.
func cosSin(a float64) (float64, float64) {
	return math.Sin(a + math.Pi/2), math.Sin(a)
}

// bitReverse puts the complex values of x in bit reversed order.
func bitReverse(x []int16) {
	n := len(x) / 2
	j := 0
	for i := 1; i < n-1; i++ {
		k := n >> 1
		for k <= j {
			j -= k
			k >>= 1
		}
		j += k
		if i < j {
			x[2*i], x[2*j] = x[2*j], x[2*i]
			x[2*i+1], x[2*j+1] = x[2*j+1], x[2*i+1]
		}
	}
}

// FFTFloat32 is an FFT for complex float32 values. The result is not scaled,
// as in esp-dsp dsps_fft2r_fc32 and dsps_fft4r_fc32. On the ESP32-S3 it uses
// the radix 4 kernel for powers of 4 and the radix 2 kernel otherwise.
type FFTFloat32 struct {
	n  int
	w2 []float32 // radix 2 twiddles in bit reversed order, as dsps_gen_w_r2_fc32
	w4 []float32 // natural order twiddles for n points, as dsps_fft4r_init_fc32

	// Byte offset pairs for the esp-dsp bit reverse lookup kernel, for n points.
	rev2, rev4 []uint16

	scratch []float32
	tw      [3][]float32 // cached twiddles for the DCT, see twiddles
}

// NewFFTFloat32 returns an FFT for up to n complex values. n must be a power of
// two and at least 2.
func NewFFTFloat32(n int) *FFTFloat32 {
	if n < 2 || n&(n-1) != 0 {
		panic("dsp: FFT size must be a power of two")
	}
	f := &FFTFloat32{n: n, w2: alignedFloat32(n), w4: alignedFloat32(2 * n)}
	for i := 0; i < n; i++ {
		c, s := cosSin(2 * math.Pi * float64(i) / float64(n))
		f.w4[2*i], f.w4[2*i+1] = float32(c), float32(s)
		if i < n/2 {
			f.w2[2*i], f.w2[2*i+1] = float32(c), float32(s)
		}
	}
	bitReverseFloat32(f.w2)
	if n <= 8192 {
		bits := bitsOf(n)
		f.rev2 = revPairs(n, func(i int) int { return reverseBits(i, bits) })
		if bits%2 == 0 {
			f.rev4 = revPairs(n, func(i int) int { return reverseDigits4(i, bits/2) })
		}
	}
	return f
}

// Transform replaces x with its discrete Fourier transform. x holds the
// complex values as real and imaginary pairs. The number of values must be a
// power of two and at most the size of f.
func (f *FFTFloat32) Transform(x []float32) {
	n := len(x) / 2
	if len(x)%2 != 0 || n < 1 || n&(n-1) != 0 || n > f.n {
		panic("dsp: FFT length")
	}
	if n == 1 {
		return
	}
	fftFloat32(f, x)
}

// TransformReal replaces the real values in x with their spectrum. x[0] is the
// DC value, x[1] the value at half the sample rate, and x[2k], x[2k+1] the
// complex bin k for k from 1 to len(x)/2-1. len(x) must be a power of two of at
// least 4 and at most the size of f.
func (f *FFTFloat32) TransformReal(x []float32) {
	n := len(x) / 2
	if n < 2 || n&(n-1) != 0 || 2*n > f.n {
		panic("dsp: FFT length")
	}
	f.Transform(x)
	step := f.n / (2 * n)
	zr, zi := x[0], x[1]
	x[0], x[1] = zr+zi, zr-zi
	for k := 1; k <= n/2; k++ {
		ar, ai := x[2*k], x[2*k+1]
		br, bi := x[2*(n-k)], -x[2*(n-k)+1]
		er, ei := (ar+br)/2, (ai+bi)/2
		or, oi := (ai-bi)/2, -(ar-br)/2
		c, s := f.w4[2*k*step], -f.w4[2*k*step+1]
		tr, ti := c*or-s*oi, c*oi+s*or
		x[2*k], x[2*k+1] = er+tr, ei+ti
		if k != n-k {
			x[2*(n-k)], x[2*(n-k)+1] = er-tr, -(ei - ti)
		}
	}
}

// fft2rFloat32Go is esp-dsp dsps_fft2r_fc32_ansi. The result is in bit
// reversed order.
func fft2rFloat32Go(x, w []float32) {
	n := len(x) / 2
	ie := 1
	for n2 := n / 2; n2 > 0; n2 >>= 1 {
		ia := 0
		for j := 0; j < ie; j++ {
			c, s := w[2*j], w[2*j+1]
			for i := 0; i < n2; i++ {
				m := ia + n2
				re := c*x[2*m] + s*x[2*m+1]
				im := c*x[2*m+1] - s*x[2*m]
				x[2*m], x[2*m+1] = x[2*ia]-re, x[2*ia+1]-im
				x[2*ia], x[2*ia+1] = x[2*ia]+re, x[2*ia+1]+im
				ia++
			}
			ia += n2
		}
		ie <<= 1
	}
}

func fftFloat32Go(f *FFTFloat32, x []float32) {
	fft2rFloat32Go(x, f.w2)
	bitReverseFloat32(x)
}

// revPairs returns the byte offsets of the complex values that rev swaps.
func revPairs(n int, rev func(int) int) []uint16 {
	var pairs []uint16
	for i := 0; i < n; i++ {
		if j := rev(i); i < j {
			pairs = append(pairs, uint16(i*8), uint16(j*8))
		}
	}
	return pairs
}

func bitsOf(n int) int {
	b := 0
	for 1<<b < n {
		b++
	}
	return b
}

func reverseBits(v, bits int) int {
	r := 0
	for i := 0; i < bits; i++ {
		r = r<<1 | v&1
		v >>= 1
	}
	return r
}

func reverseDigits4(v, digits int) int {
	r := 0
	for i := 0; i < digits; i++ {
		r = r<<2 | v&3
		v >>= 2
	}
	return r
}

func bitReverseFloat32(x []float32) {
	n := len(x) / 2
	j := 0
	for i := 1; i < n-1; i++ {
		k := n >> 1
		for k <= j {
			j -= k
			k >>= 1
		}
		j += k
		if i < j {
			x[2*i], x[2*j] = x[2*j], x[2*i]
			x[2*i+1], x[2*j+1] = x[2*j+1], x[2*i+1]
		}
	}
}

// TransformReal replaces the real values in x with their spectrum divided by
// len(x), in the same layout as FFTFloat32.TransformReal. len(x) must be a
// power of two of at least 4 and at most the size of f.
func (f *FFTInt16) TransformReal(x []int16) {
	n := len(x) / 2
	if n < 2 || n&(n-1) != 0 || 2*n > len(f.w) {
		panic("dsp: FFT length")
	}
	f.Transform(x)
	bits := bitsOf(len(f.w) / 2)
	step := len(f.w) / (2 * n)
	zr, zi := int32(x[0]), int32(x[1])
	x[0], x[1] = int16((zr+zi)>>1), int16((zr-zi)>>1)
	for k := 1; k <= n/2; k++ {
		ar, ai := int32(x[2*k]), int32(x[2*k+1])
		br, bi := int32(x[2*(n-k)]), -int32(x[2*(n-k)+1])
		er, ei := ar+br, ai+bi
		or, oi := ai-bi, -(ar - br)
		j := reverseBits(k*step, bits)
		c, s := int32(f.w[2*j]), -int32(f.w[2*j+1])
		tr := (c*or - s*oi) >> 15
		ti := (c*oi + s*or) >> 15
		x[2*k], x[2*k+1] = int16((er+tr)>>2), int16((ei+ti)>>2)
		if k != n-k {
			x[2*(n-k)], x[2*(n-k)+1] = int16((er-tr)>>2), int16(-(ei-ti)>>2)
		}
	}
}
