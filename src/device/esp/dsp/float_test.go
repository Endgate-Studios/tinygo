package dsp

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

func dft(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := range out {
		for j, v := range x {
			out[k] += v * cmplx.Exp(complex(0, -2*math.Pi*float64(j*k)/float64(n)))
		}
	}
	return out
}

func TestIntVectors(t *testing.T) {
	d16 := make([]int16, 2)
	SubInt16(d16, []int16{-32768, 5}, []int16{1, 7})
	if d16[0] != -32768 || d16[1] != -2 {
		t.Errorf("SubInt16: %v", d16)
	}
	d8 := make([]int8, 2)
	AddInt8(d8, []int8{127, -3}, []int8{1, 1})
	if d8[0] != 127 || d8[1] != -2 {
		t.Errorf("AddInt8: %v", d8)
	}
	SubInt8(d8, []int8{-128, 3}, []int8{1, 1})
	if d8[0] != -128 || d8[1] != 2 {
		t.Errorf("SubInt8: %v", d8)
	}
	MulInt8(d8, []int8{-128, 100}, []int8{-128, 100}, 7)
	if d8[0] != -128 || d8[1] != int8(10000>>7) {
		t.Errorf("MulInt8: %v", d8)
	}
	if v := DotUint16([]uint16{65535, 2}, []uint16{65535, 3}); v != 65535*65535+6 {
		t.Errorf("DotUint16: %d", v)
	}
	if v := DotUint8([]uint8{255, 2}, []uint8{255, 3}); v != 255*255+6 {
		t.Errorf("DotUint8: %d", v)
	}
	x := []int16{1, 2, 3, 4, 5, 6, 7}
	dst := make([]int16, 3)
	FIRDecimInt16(dst, x, []int16{1, 1}, 2, 0)
	if dst[0] != 3 || dst[1] != 7 || dst[2] != 11 {
		t.Errorf("FIRDecimInt16: %v", dst)
	}
	b := make([]byte, 40)
	Fill(b, 7)
	c := make([]byte, 40)
	if Copy(c, b) != 40 || c[39] != 7 || c[0] != 7 {
		t.Errorf("Copy or Fill: %v", c)
	}
}

func TestDot2D(t *testing.T) {
	a := []int16{1, 2, 99, 3, 4, 99}
	b := []int16{5, 6, 7, 8}
	// Region 2 by 2, a has stride 3, b has stride 2.
	if v := Dot2DInt16(a, 3, b, 2, 2, 2, 0); v != 1*5+2*6+3*7+4*8 {
		t.Errorf("Dot2DInt16: %d", v)
	}
	if v := Dot2DInt16(a, 3, b, 2, 2, 2, 10); v != 1*15+2*16+3*17+4*18 {
		t.Errorf("Dot2DInt16 offset: %d", v)
	}
	au := []uint8{1, 2, 3, 4}
	if v := Dot2DUint8(au, 2, au, 2, 2, 2, -1); v != 1*0+2*1+3*2+4*3 {
		t.Errorf("Dot2DUint8 offset: %d", v)
	}
}

func TestFloatBasics(t *testing.T) {
	if v := DotFloat32([]float32{1, 2, 3}, []float32{4, 5, 6}); v != 32 {
		t.Errorf("DotFloat32: %v", v)
	}
	dst := make([]float32, 3)
	FIRDecimFloat32(dst, []float32{1, 2, 3, 4, 5, 6, 7}, []float32{1, 1}, 2)
	if dst[0] != 3 || dst[1] != 7 || dst[2] != 11 {
		t.Errorf("FIRDecimFloat32: %v", dst)
	}
	m := make([]float32, 4)
	MatMulFloat32(m, []float32{1, 2, 3, 4, 5, 6}, []float32{7, 8, 9, 10, 11, 12}, 2, 3, 2)
	if m[0] != 58 || m[1] != 64 || m[2] != 139 || m[3] != 154 {
		t.Errorf("MatMulFloat32: %v", m)
	}
	conv := make([]float32, 4)
	ConvFloat32(conv, []float32{1, 2, 3}, []float32{1, 1})
	if conv[0] != 1 || conv[1] != 3 || conv[2] != 5 || conv[3] != 3 {
		t.Errorf("ConvFloat32: %v", conv)
	}
	cc := make([]float32, 4)
	CCorrFloat32(cc, []float32{1, 2, 3}, []float32{1, 1})
	if cc[0] != 1 || cc[1] != 3 || cc[2] != 5 || cc[3] != 3 {
		t.Errorf("CCorrFloat32: %v", cc)
	}
	w := make([]float32, 5)
	WindowHann(w)
	if w[0] != 0 || w[2] != 1 || w[4] > 1e-6 {
		t.Errorf("WindowHann: %v", w)
	}
}

func TestFFTFloat32(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	f := NewFFTFloat32(64)
	for _, n := range []int{1, 2, 8, 64} {
		x := make([]float32, 2*n)
		c := make([]complex128, n)
		for i := 0; i < n; i++ {
			x[2*i], x[2*i+1] = r.Float32(), r.Float32()
			c[i] = complex(float64(x[2*i]), float64(x[2*i+1]))
		}
		f.Transform(x)
		want := dft(c)
		for i := 0; i < n; i++ {
			if cmplx.Abs(complex(float64(x[2*i]), float64(x[2*i+1]))-want[i]) > 1e-4 {
				t.Fatalf("n=%d bin %d: got %v%+vi, want %v", n, i, x[2*i], x[2*i+1], want[i])
			}
		}
	}
	// Real input of 64 values.
	x := make([]float32, 64)
	c := make([]complex128, 64)
	for i := range x {
		x[i] = r.Float32() - 0.5
		c[i] = complex(float64(x[i]), 0)
	}
	f.TransformReal(x)
	want := dft(c)
	check := func(k int, re, im float32) {
		if cmplx.Abs(complex(float64(re), float64(im))-want[k]) > 1e-4 {
			t.Errorf("real bin %d: got %v%+vi, want %v", k, re, im, want[k])
		}
	}
	check(0, x[0], 0)
	check(32, x[1], 0)
	for k := 1; k < 32; k++ {
		check(k, x[2*k], x[2*k+1])
	}
}

func TestFFTInt16Real(t *testing.T) {
	f := NewFFTInt16(256)
	x := AlignedInt16(128)
	for i := range x {
		x[i] = int16(16000 * math.Cos(2*math.Pi*9*float64(i)/128))
	}
	f.TransformReal(x)
	// A cosine of amplitude 16000 gives 16000*128/2 in bin 9, divided by 128.
	for k := 1; k < 64; k++ {
		re, im := int(x[2*k]), int(x[2*k+1])
		want := 0
		if k == 9 {
			want = 8000
		}
		if re < want-6 || re > want+6 || im < -6 || im > 6 {
			t.Errorf("bin %d: got %d%+di, want %d", k, re, im, want)
		}
	}
}

func TestBiquad(t *testing.T) {
	// A low pass at 0.05 passes DC and blocks 0.4.
	f := NewLowPassFloat32(0.05, 0.707)
	in := make([]float32, 400)
	out := make([]float32, 400)
	for i := range in {
		in[i] = 1
	}
	f.Process(out, in)
	if math.Abs(float64(out[399])-1) > 1e-3 {
		t.Errorf("low pass DC gain: %v", out[399])
	}
	f.Reset()
	ToneFloat32(in, 1, 0.4, 0)
	f.Process(out, in)
	var peak float64
	for _, v := range out[200:] {
		peak = math.Max(peak, math.Abs(float64(v)))
	}
	if peak > 0.02 {
		t.Errorf("low pass stop band: %v", peak)
	}
}

func TestSNR(t *testing.T) {
	x := make([]float32, 1024)
	ToneFloat32(x, 1, 0.1, 0)
	r := rand.New(rand.NewSource(3))
	for i := range x {
		x[i] += (r.Float32() - 0.5) * 0.002
	}
	if snr := SNRFloat32(x, false); snr < 50 || snr > 80 {
		t.Errorf("SNR: %v", snr)
	}
	if sfdr := SFDRFloat32(x, false); sfdr < 40 {
		t.Errorf("SFDR: %v", sfdr)
	}
}
