package dsp

import (
	"io"
)

// View writes a text plot of data to w, as esp-dsp dsps_view. The plot is
// width columns by height rows, for values from min to max.
func View(w io.Writer, data []float32, width, height int, min, max float32, mark byte) {
	if width < 1 || height < 2 || max <= min {
		panic("dsp: bad view size")
	}
	grid := make([]byte, width*height)
	for i := range grid {
		grid[i] = ' '
	}
	lo := make([]float32, width)
	hi := make([]float32, width)
	for i := range lo {
		lo[i], hi[i] = max, min
	}
	xStep := float32(width) / float32(len(data))
	yStep := float32(height-1) / (max - min)
	for i, v := range data {
		x := int(float32(i) * xStep)
		if v < lo[x] {
			lo[x] = v
		}
		if v > hi[x] {
			hi[x] = v
		}
		if lo[x] < min {
			lo[x] = min
		}
		if hi[x] > max {
			hi[x] = max
		}
	}
	for x := 0; x < width; x++ {
		count := int((hi[x]-lo[x])*yStep) + 1
		for y := 0; y < count; y++ {
			pos := int((max-hi[x])*yStep) + y
			if pos >= 0 && pos < height {
				grid[pos*width+x] = mark
			}
		}
	}
	line := make([]byte, 0, width+3)
	line = append(line, ' ')
	for x := 0; x < width; x++ {
		line = append(line, '_')
	}
	w.Write(append(line, '\n'))
	for y := 0; y < height; y++ {
		line = append(line[:0], byte('0'+y%10))
		line = append(line, grid[y*width:(y+1)*width]...)
		w.Write(append(line, '|', '\n'))
	}
	line = append(line[:0], ' ')
	for x := 0; x < width; x++ {
		line = append(line, byte('0'+x%10))
	}
	w.Write(append(line, '\n'))
}

// ViewInt16 is View for Q15 values, as esp-dsp dsps_view_s16.
func ViewInt16(w io.Writer, data []int16, width, height int, min, max float32, mark byte) {
	f := make([]float32, len(data))
	for i, v := range data {
		f[i] = float32(v) / 32768
	}
	View(w, f, width, height, min, max, mark)
}

// ViewSpectrum writes a 64 by 10 plot of a spectrum in dB, as esp-dsp
// dsps_view_spectrum.
func ViewSpectrum(w io.Writer, data []float32, min, max float32) {
	View(w, data, 64, 10, min, max, '|')
}
