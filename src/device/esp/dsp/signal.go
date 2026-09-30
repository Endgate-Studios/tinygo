package dsp

import (
	"math"
)

// ImpulseFloat32 sets dst to zero with a 1 at pos, as esp-dsp dsps_d_gen_f32.
func ImpulseFloat32(dst []float32, pos int) {
	if pos < 0 || pos >= len(dst) {
		panic("dsp: position out of range")
	}
	for i := range dst {
		dst[i] = 0
	}
	dst[pos] = 1
}

// StepFloat32 sets dst to 0 before pos and to 1 from pos, as esp-dsp
// dsps_h_gen_f32.
func StepFloat32(dst []float32, pos int) {
	if pos < 0 || pos >= len(dst) {
		panic("dsp: position out of range")
	}
	for i := range dst {
		if i < pos {
			dst[i] = 0
		} else {
			dst[i] = 1
		}
	}
}

// ComplexGenerator makes a complex tone from a sine table, as esp-dsp
// dsps_cplx_gen. freq is in cycles per sample and phase in cycles, both
// between -1 and 1.
type ComplexGenerator struct {
	lut   []float32
	lut16 []int16
	Freq  float32
	Phase float32
}

// NewComplexGenerator returns a generator with a sine table of lutLen values.
// lutLen must be a power of two from 256 to 8192.
func NewComplexGenerator(lutLen int, freq, phase float32) *ComplexGenerator {
	if lutLen < 256 || lutLen > 8192 || lutLen&(lutLen-1) != 0 {
		panic("dsp: table length")
	}
	g := &ComplexGenerator{lut: make([]float32, lutLen), lut16: make([]int16, lutLen), Freq: freq, Phase: phase}
	for i := range g.lut {
		s := math.Sin(2 * math.Pi * float64(i) / float64(lutLen))
		g.lut[i] = float32(s)
		g.lut16[i] = int16(s * math.MaxInt16)
	}
	return g
}

// Float32 writes len(dst)/2 complex values as real and imaginary pairs.
func (g *ComplexGenerator) Float32(dst []float32) {
	n := len(g.lut)
	for i := 0; i < len(dst)/2; i++ {
		s, c := g.next(n)
		dst[2*i], dst[2*i+1] = g.lut[c], g.lut[s]
	}
}

// Int16 writes len(dst)/2 complex Q15 values as real and imaginary pairs.
func (g *ComplexGenerator) Int16(dst []int16) {
	n := len(g.lut16)
	for i := 0; i < len(dst)/2; i++ {
		s, c := g.next(n)
		dst[2*i], dst[2*i+1] = g.lut16[c], g.lut16[s]
	}
}

func (g *ComplexGenerator) next(n int) (sinPos, cosPos int) {
	if g.Phase < 0 {
		g.Phase++
	}
	if g.Phase >= 1 {
		g.Phase--
	}
	sinPos = int(g.Phase * float32(n))
	cosPos = (sinPos + n/4) & (n - 1)
	g.Phase += g.Freq
	return sinPos, cosPos
}

// ToneFloat32 fills dst with amplitude*sin(2*pi*freq*i + phase). freq is
// in cycles per sample and phase in degrees, as in esp-dsp dsps_tone_gen.
func ToneFloat32(dst []float32, amplitude, freq, phase float32) {
	ph := float64(phase) / 180 * math.Pi
	w := 2 * math.Pi * float64(freq)
	for i := range dst {
		dst[i] = amplitude * float32(math.Sin(w*float64(i)+ph))
	}
}
