package dsp

import (
	"math"
)

// SNRFloat32 returns the signal to noise ratio of x in dB, with the method of
// esp-dsp dsps_snr_f32: a Hann window, the largest bin and 7 bins on each side
// as signal, and a 2 dB correction. The first 7 bins are left out unless useDC
// is true. len(x) must be a power of two.
func SNRFloat32(x []float32, useDC bool) float32 {
	return snr(spectrumPower(x, nil), useDC)
}

// SNRComplexFloat32 is SNRFloat32 for len(x)/2 complex values.
func SNRComplexFloat32(x []float32) float32 {
	return snr(spectrumPower(nil, x), true)
}

// SFDRFloat32 returns the spurious free dynamic range of x in dB, with the
// method of esp-dsp dsps_sfdr_f32: a Hann window, and the smallest distance of
// the largest bin to the bins outside of 5 bins on each side of it.
func SFDRFloat32(x []float32, useDC bool) float32 {
	return sfdr(spectrumPower(x, nil), useDC)
}

// SFDRComplexFloat32 is SFDRFloat32 for len(x)/2 complex values.
func SFDRComplexFloat32(x []float32) float32 {
	return sfdr(spectrumPower(nil, x), true)
}

// spectrumPower returns the power of the bins of the real values re or of the
// complex values cplx, with a Hann window. Real input gives len/2 bins.
func spectrumPower(re, cplx []float32) []float64 {
	n := len(re)
	if cplx != nil {
		n = len(cplx) / 2
	}
	if n < 2 || n&(n-1) != 0 {
		panic("dsp: length must be a power of two")
	}
	buf := make([]float32, 2*n)
	for i := 0; i < n; i++ {
		c, _ := cosSin(float64(i) * 2 * math.Pi / float64(n))
		w := float32(0.5 * (1 - c))
		if cplx != nil {
			buf[2*i], buf[2*i+1] = cplx[2*i]*w, cplx[2*i+1]*w
		} else {
			buf[2*i] = re[i] * w
		}
	}
	NewFFTFloat32(n).Transform(buf)
	bins := n
	if cplx == nil {
		bins = n / 2
	}
	p := make([]float64, bins)
	for i := range p {
		p[i] = float64(buf[2*i])*float64(buf[2*i]) + float64(buf[2*i+1])*float64(buf[2*i+1])
	}
	return p
}

func peak(p []float64) int {
	pos := 0
	for i, v := range p {
		if v > p[pos] {
			pos = i
		}
	}
	return pos
}

func snr(p []float64, useDC bool) float32 {
	const width = 7
	pos := peak(p)
	start := 0
	if !useDC {
		start = width
	}
	var noise float64
	for i := start; i < len(p); i++ {
		if i < pos-width || i > pos+width {
			noise += p[i]
		}
	}
	noise += math.SmallestNonzeroFloat32
	if noise < p[pos]*1e-11 {
		return 192
	}
	return float32(10*math.Log10(p[pos]/noise) - 2)
}

func sfdr(p []float64, useDC bool) float32 {
	const width = 5
	pos := peak(p)
	start := 0
	if !useDC {
		start = width
	}
	maxDB := 10 * math.Log10(p[pos])
	minDiff := math.MaxFloat32
	for i := start; i < len(p); i++ {
		if i < pos-width || i > pos+width {
			minDiff = math.Min(minDiff, maxDB-10*math.Log10(p[i]))
		}
	}
	return float32(minDiff)
}
