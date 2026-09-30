package dsp

// ConvFloat32 sets dst to the full convolution of signal and kernel.
// len(dst) must be len(signal)+len(kernel)-1.
func ConvFloat32(dst, signal, kernel []float32) {
	if len(signal) == 0 || len(kernel) == 0 || len(dst) != len(signal)+len(kernel)-1 {
		panic("dsp: length mismatch")
	}
	for i := range dst {
		var sum float32
		lo := max(0, i-len(kernel)+1)
		hi := min(i, len(signal)-1)
		for j := lo; j <= hi; j++ {
			sum += signal[j] * kernel[i-j]
		}
		dst[i] = sum
	}
}

// CorrFloat32 sets dst[n] to the sum of signal[n+m]*pattern[m], as esp-dsp
// dsps_corr does. len(dst) must be len(signal)-len(pattern)+1.
func CorrFloat32(dst, signal, pattern []float32) {
	FIRFloat32(dst, signal, pattern)
}

// CCorrFloat32 sets dst to the full cross correlation of a and b, as esp-dsp
// dsps_ccorr does. len(dst) must be len(a)+len(b)-1.
func CCorrFloat32(dst, a, b []float32) {
	if len(a) == 0 || len(b) == 0 || len(dst) != len(a)+len(b)-1 {
		panic("dsp: length mismatch")
	}
	for i := range dst {
		lag := i - (len(b) - 1)
		var sum float32
		for j := range b {
			if k := j + lag; k >= 0 && k < len(a) {
				sum += a[k] * b[j]
			}
		}
		dst[i] = sum
	}
}
