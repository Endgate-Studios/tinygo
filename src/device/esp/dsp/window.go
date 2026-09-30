package dsp

import (
	"math"
)

// The window functions fill w with the window of that name, with the formulas
// of esp-dsp dsps_wind. The denominator is len(w)-1.

func WindowHann(w []float32) {
	cosineWindow(w, 0.5, 0.5)
}

func WindowBlackman(w []float32) {
	cosineWindow(w, 0.42, 0.5, 0.08)
}

func WindowBlackmanHarris(w []float32) {
	cosineWindow(w, 0.35875, 0.48829, 0.14128, 0.01168)
}

func WindowBlackmanNuttall(w []float32) {
	cosineWindow(w, 0.3635819, 0.4891775, 0.1365995, 0.0106411)
}

func WindowNuttall(w []float32) {
	cosineWindow(w, 0.355768, 0.487396, 0.144232, 0.012604)
}

func WindowFlatTop(w []float32) {
	cosineWindow(w, 1, 1.93, 1.29, 0.388, 0.028)
}

// cosineWindow sets w[i] to the sum of (-1)^k * a[k] * cos(2*pi*k*i/(len(w)-1)).
func cosineWindow(w []float32, a ...float64) {
	if len(w) == 1 {
		w[0] = 1
		return
	}
	d := float64(len(w) - 1)
	for i := range w {
		var v float64
		for k, ak := range a {
			term := ak * math.Cos(2*math.Pi*float64(k)*float64(i)/d)
			if k%2 == 1 {
				term = -term
			}
			v += term
		}
		w[i] = float32(v)
	}
}
