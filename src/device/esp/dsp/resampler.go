package dsp

// ResamplerFloat32 changes the sample rate with a multi-rate FIR filter, as
// esp-dsp dsps_resampler_mr. It switches between two decimation factors, so the
// average output rate follows the input rate divided by factor.
type ResamplerFloat32 struct {
	fir            *FIRMultiRateFloat32
	factor         float32
	decimF, decimC int
	avgIn, avgOut  float32
}

// NewResamplerFloat32 returns a resampler. coeffs is the filter for interp,
// and factor is the input rate divided by the output rate. It must be 1 or more.
func NewResamplerFloat32(coeffs []float32, interp int, factor float32) *ResamplerFloat32 {
	if factor < 1 {
		panic("dsp: bad resampler factor")
	}
	d := float32(interp) / factor
	r := &ResamplerFloat32{factor: factor, decimF: int(d), decimC: int(d), avgIn: 1, avgOut: factor}
	if float32(r.decimC) < d {
		r.decimC++
	}
	r.fir = NewFIRMultiRateFloat32(coeffs, interp, r.decimC, 0)
	return r
}

// Process resamples src into dst and returns the number of outputs.
// correction is subtracted from the output count for the rate control.
func (r *ResamplerFloat32) Process(dst, src []float32, correction int) int {
	n := r.fir.Process(dst, src)
	const k = 0.9999
	r.avgIn = r.avgIn*k + float32(len(src))*(1-k)
	r.avgOut = r.avgOut*k + float32(n-correction)*(1-k)
	if r.avgIn*r.factor > r.avgOut {
		r.fir.Decim = r.decimF
	} else {
		r.fir.Decim = r.decimC
	}
	return n
}

// PhaseResamplerFloat32 changes the sample rate with cubic interpolation, as
// esp-dsp dsps_resampler_ph.
type PhaseResamplerFloat32 struct {
	delay [4]float32
	pos   int
	step  float32
	phase float32
}

// NewPhaseResamplerFloat32 returns a resampler for the given factor, the
// output rate divided by the input rate. It must be 1 or more.
func NewPhaseResamplerFloat32(factor float32) *PhaseResamplerFloat32 {
	if factor < 1 {
		panic("dsp: bad resampler factor")
	}
	return &PhaseResamplerFloat32{step: 1 / factor}
}

// Process resamples src into dst and returns the number of outputs. dst needs
// space for len(src)*factor+1 values.
func (r *PhaseResamplerFloat32) Process(dst, src []float32) int {
	var x [4]float32
	load := func() {
		for i := range x {
			x[i] = r.delay[(r.pos+i)%4]
		}
	}
	load()
	out, in := 0, 0
	for in < len(src) {
		c0 := x[1]
		c1 := 0.5 * (x[2] - x[0])
		c2 := x[0] - 2.5*x[1] + 2*x[2] - 0.5*x[3]
		c3 := 0.5*(x[3]-x[0]) + 1.5*(x[1]-x[2])
		dst[out] = c0 + r.phase*(c1+r.phase*(c2+r.phase*c3))
		out++
		r.phase += r.step
		for r.phase >= 1 && in < len(src) {
			r.phase--
			r.delay[r.pos] = src[in]
			r.pos = (r.pos + 1) % 4
			in++
			load()
		}
	}
	return out
}

// ResamplerInt16 is ResamplerFloat32 for int16 values, as esp-dsp
// dsps_resampler_mr with fixed_point set. The filter result is shifted right
// by 15-shift.
type ResamplerInt16 struct {
	fir            *FIRMultiRateInt16
	factor         float32
	decimF, decimC int
	avgIn, avgOut  float32
}

// NewResamplerInt16 returns a resampler, as NewResamplerFloat32.
func NewResamplerInt16(coeffs []int16, interp int, factor float32, shift int) *ResamplerInt16 {
	if factor < 1 {
		panic("dsp: bad resampler factor")
	}
	d := float32(interp) / factor
	r := &ResamplerInt16{factor: factor, decimF: int(d), decimC: int(d), avgIn: 1, avgOut: factor}
	if float32(r.decimC) < d {
		r.decimC++
	}
	r.fir = NewFIRMultiRateInt16(coeffs, interp, r.decimC, 0, shift)
	return r
}

// Process resamples src into dst and returns the number of outputs.
func (r *ResamplerInt16) Process(dst, src []int16, correction int) int {
	n := r.fir.Process(dst, src)
	const k = 0.9999
	r.avgIn = r.avgIn*k + float32(len(src))*(1-k)
	r.avgOut = r.avgOut*k + float32(n-correction)*(1-k)
	if r.avgIn*r.factor > r.avgOut {
		r.fir.Decim = r.decimF
	} else {
		r.fir.Decim = r.decimC
	}
	return n
}
