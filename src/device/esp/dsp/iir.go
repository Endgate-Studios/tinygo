package dsp

import (
	"math"
)

// BiquadFloat32 is a second order IIR filter in direct form II, as esp-dsp
// dsps_biquad_f32. The coefficients are normalized so that a0 is 1.
type BiquadFloat32 struct {
	B0, B1, B2 float32
	A1, A2     float32
	w1, w2     float32
}

// Process filters src into dst. dst and src can be the same slice.
func (f *BiquadFloat32) Process(dst, src []float32) {
	checkLen2(len(dst), len(src))
	biquadFloat32(f, dst, src)
}

// biquadFloat32Go is esp-dsp dsps_biquad_f32_ansi.
func biquadFloat32Go(f *BiquadFloat32, dst, src []float32) {
	for i, x := range src {
		w := x - f.A1*f.w1 - f.A2*f.w2
		dst[i] = f.B0*w + f.B1*f.w1 + f.B2*f.w2
		f.w2 = f.w1
		f.w1 = w
	}
}

// Reset clears the state of the filter.
func (f *BiquadFloat32) Reset() {
	f.w1, f.w2 = 0, 0
}

// The New functions design the filter types of esp-dsp dsps_biquad_gen. freq
// is the frequency in cycles per sample (0 to 0.5), q is the quality factor
// and gain is in dB. See the Audio EQ Cookbook by Robert Bristow-Johnson.

func NewLowPassFloat32(freq, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	return newBiquad((1-c)/2, 1-c, (1-c)/2, 1+alpha, -2*c, 1-alpha)
}

func NewHighPassFloat32(freq, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	return newBiquad((1+c)/2, -(1 + c), (1+c)/2, 1+alpha, -2*c, 1-alpha)
}

// NewBandPassFloat32 has a peak gain of 0 dB.
func NewBandPassFloat32(freq, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	return newBiquad(alpha, 0, -alpha, 1+alpha, -2*c, 1-alpha)
}

func NewNotchFloat32(freq, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	return newBiquad(1, -2*c, 1, 1+alpha, -2*c, 1-alpha)
}

func NewAllPassFloat32(freq, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	return newBiquad(1-alpha, -2*c, 1+alpha, 1+alpha, -2*c, 1-alpha)
}

func NewPeakingFloat32(freq, q, gain float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	a := math.Pow(10, float64(gain)/40)
	return newBiquad(1+alpha*a, -2*c, 1-alpha*a, 1+alpha/a, -2*c, 1-alpha/a)
}

func NewLowShelfFloat32(freq, q, gain float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	a := math.Pow(10, float64(gain)/40)
	s := 2 * math.Sqrt(a) * alpha
	return newBiquad(a*((a+1)-(a-1)*c+s), 2*a*((a-1)-(a+1)*c), a*((a+1)-(a-1)*c-s),
		(a+1)+(a-1)*c+s, -2*((a-1)+(a+1)*c), (a+1)+(a-1)*c-s)
}

func NewHighShelfFloat32(freq, q, gain float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	a := math.Pow(10, float64(gain)/40)
	s := 2 * math.Sqrt(a) * alpha
	return newBiquad(a*((a+1)+(a-1)*c+s), -2*a*((a-1)+(a+1)*c), a*((a+1)+(a-1)*c-s),
		(a+1)-(a-1)*c+s, 2*((a-1)-(a+1)*c), (a+1)-(a-1)*c-s)
}

// NewBandPassSkirtFloat32 has a constant skirt gain, as esp-dsp
// dsps_biquad_gen_bpf_f32. The peak gain is q.
func NewBandPassSkirtFloat32(freq, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, s := cosSin(w)
	return newBiquad(s/2, 0, -s/2, 1+alpha, -2*c, 1-alpha)
}

// NewNotchGainFloat32 is esp-dsp dsps_biquad_gen_notch_f32, which has a gain in dB.
func NewNotchGainFloat32(freq, gain, q float32) *BiquadFloat32 {
	w, alpha := biquadParams(freq, q)
	c, _ := cosSin(w)
	a := math.Pow(10, float64(gain)/40)
	return newBiquad(1+alpha*a, -2*c, 1-alpha*a, 1+alpha, -2*c, 1-alpha)
}

// NewAllPass180Float32 and NewAllPass360Float32 are the esp-dsp all pass
// designs. esp-dsp uses the same coefficients for both.
func NewAllPass180Float32(freq, q float32) *BiquadFloat32 { return NewAllPassFloat32(freq, q) }

func NewAllPass360Float32(freq, q float32) *BiquadFloat32 { return NewAllPassFloat32(freq, q) }

// BiquadStereoFloat32 filters two interleaved channels with the same
// coefficients, as esp-dsp dsps_biquad_sf32.
type BiquadStereoFloat32 struct {
	BiquadFloat32
	w [4]float32
}

// NewBiquadStereoFloat32 returns a stereo filter with the coefficients of f.
func NewBiquadStereoFloat32(f *BiquadFloat32) *BiquadStereoFloat32 {
	s := &BiquadStereoFloat32{BiquadFloat32: *f}
	s.BiquadFloat32.Reset()
	return s
}

// Process filters the left and right values of src into dst.
func (f *BiquadStereoFloat32) Process(dst, src []float32) {
	if len(src)%2 != 0 {
		panic("dsp: length must be even")
	}
	checkLen2(len(dst), len(src))
	for i := 0; i < len(src); i += 2 {
		for ch := 0; ch < 2; ch++ {
			w0, w1 := f.w[2*ch], f.w[2*ch+1]
			d := src[i+ch] - f.A1*w0 - f.A2*w1
			dst[i+ch] = f.B0*d + f.B1*w0 + f.B2*w1
			f.w[2*ch], f.w[2*ch+1] = d, w0
		}
	}
}

// Reset clears the state of both channels.
func (f *BiquadStereoFloat32) Reset() {
	f.w = [4]float32{}
}

// biquadParams clamps q to at least 0.0001, as esp-dsp does.
func biquadParams(freq, q float32) (w, alpha float64) {
	if q <= 0.0001 {
		q = 0.0001
	}
	w = 2 * math.Pi * float64(freq)
	_, s := cosSin(w)
	return w, s / (2 * float64(q))
}

func newBiquad(b0, b1, b2, a0, a1, a2 float64) *BiquadFloat32 {
	return &BiquadFloat32{
		B0: float32(b0 / a0), B1: float32(b1 / a0), B2: float32(b2 / a0),
		A1: float32(a1 / a0), A2: float32(a2 / a0),
	}
}

// NewPeakingEQFloat32 is esp-dsp dsps_biquad_gen_peakingEQ_f32, which is the
// band pass of NewBandPassFloat32. For a peaking filter with a gain, use
// NewPeakingFloat32.
func NewPeakingEQFloat32(freq, q float32) *BiquadFloat32 {
	return NewBandPassFloat32(freq, q)
}
