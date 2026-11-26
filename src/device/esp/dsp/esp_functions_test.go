package dsp

import (
	"bytes"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

type transformCheck struct {
	name string
	run  func([]float32)
	want []float64
}

func TestTransforms(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	f := NewFFTFloat32(64)
	for _, n := range []int{2, 4, 16, 64} {
		x := make([]float32, n)
		for i := range x {
			x[i] = r.Float32() - 0.5
		}
		N := float64(n)
		direct := func(fn func(j, k float64) float64) []float64 {
			out := make([]float64, n)
			for k := range out {
				for j := range x {
					out[k] += float64(x[j]) * fn(float64(j), float64(k))
				}
			}
			return out
		}
		idct := make([]float64, n)
		for k := range idct {
			idct[k] = float64(x[0]) / 2
			for j := 1; j < n; j++ {
				idct[k] += float64(x[j]) * math.Cos(math.Pi*float64(j)*(float64(k)+0.5)/N)
			}
		}
		checks := []transformCheck{
			{"DCT", f.DCT, direct(func(j, k float64) float64 { return math.Cos(math.Pi * (j + 0.5) * k / N) })},
			{"DCTIV", f.DCTIV, direct(func(j, k float64) float64 { return math.Cos(math.Pi / N * (j + 0.5) * (k + 0.5)) })},
			{"DSTIV", f.DSTIV, direct(func(j, k float64) float64 { return math.Sin(math.Pi / N * (j + 0.5) * (k + 0.5)) })},
			{"IDCT", f.IDCT, idct},
		}
		for _, c := range checks {
			y := append([]float32(nil), x...)
			c.run(y)
			for k := range y {
				if !near(float64(y[k]), c.want[k], 1e-4) {
					t.Errorf("%s n=%d k=%d: got %v, want %v", c.name, n, k, y[k], c.want[k])
					break
				}
			}
		}
	}
}

func TestConv2D(t *testing.T) {
	src := []float32{1, 2, 3, 4, 5, 6, 7, 8, 9}
	k := []float32{0, 0, 0, 0, 1, 1, 0, 0, 0}
	dst := make([]float32, 9)
	Conv2DFloat32(dst, 3, src, 3, 3, 3, k, 3, 3, 3)
	want := []float32{3, 5, 3, 9, 11, 6, 15, 17, 9}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("Conv2DFloat32: %v", dst)
		}
	}
}

func TestMatrixExtras(t *testing.T) {
	a := []float32{1, 2, 0, 3, 4, 0}
	b := []float32{5, 6, 7, 8}
	d := make([]float32, 4)
	MatAddFloat32(d, a, b, 2, 2, 2, 3, 2)
	if d[0] != 6 || d[3] != 12 {
		t.Errorf("MatAddFloat32: %v", d)
	}
	MatSubFloat32(d, a, b, 2, 2, 2, 3, 2)
	if d[0] != -4 || d[3] != -4 {
		t.Errorf("MatSubFloat32: %v", d)
	}
	MatMulConstFloat32(d, a, 2, 2, 2, 2, 3)
	if d[1] != 4 || d[2] != 6 {
		t.Errorf("MatMulConstFloat32: %v", d)
	}
	var c3 [3]float32
	MatMul3x3x1Float32(&c3, &[9]float32{1, 0, 0, 0, 2, 0, 0, 0, 3}, &[3]float32{1, 1, 1})
	if c3 != [3]float32{1, 2, 3} {
		t.Errorf("MatMul3x3x1: %v", c3)
	}
	var c16 [16]float32
	id := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	m := [16]float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	MatMul4x4x4Float32(&c16, &id, &m)
	if c16 != m {
		t.Errorf("MatMul4x4x4: %v", c16)
	}
	v := make([]int32, 2)
	MatVecInt8(v, []int8{1, 2, 3, 4}, []int8{10, 1}, 2, 2)
	if v[0] != 12 || v[1] != 34 {
		t.Errorf("MatVecInt8: %v", v)
	}
}

func TestSmallFunctions(t *testing.T) {
	d := make([]int16, 2)
	MulConstInt16(d, []int16{16384, -32768}, 16384)
	if d[0] != 8192 || d[1] != -16384 {
		t.Errorf("MulConstInt16: %v", d)
	}
	if v := DotStrideFloat32([]float32{1, 9, 2, 9, 3}, 2, []float32{1, 1, 1}, 1, 3); v != 6 {
		t.Errorf("DotStrideFloat32: %v", v)
	}
	s := []int16{1, 2, 3}
	ReverseInt16(s)
	if s[0] != 3 || s[2] != 1 {
		t.Errorf("ReverseInt16: %v", s)
	}
	for _, x := range []float32{0.01, 1, 2, 100} {
		if !near(float64(SqrtApprox(x)), math.Sqrt(float64(x)), 0.07*math.Sqrt(float64(x))) {
			t.Errorf("SqrtApprox(%v) = %v", x, SqrtApprox(x))
		}
		if !near(float64(InvSqrtApprox(x)), 1/math.Sqrt(float64(x)), 0.01/math.Sqrt(float64(x))) {
			t.Errorf("InvSqrtApprox(%v) = %v", x, InvSqrtApprox(x))
		}
	}
	x := make([]float32, 4)
	ImpulseFloat32(x, 2)
	if x[2] != 1 || x[1] != 0 {
		t.Errorf("ImpulseFloat32: %v", x)
	}
	StepFloat32(x, 1)
	if x[0] != 0 || x[1] != 1 || x[3] != 1 {
		t.Errorf("StepFloat32: %v", x)
	}
	g := NewComplexGenerator(256, 0.25, 0)
	c := make([]float32, 8)
	g.Float32(c)
	want := []float32{1, 0, 0, 1, -1, 0, 0, -1}
	for i := range want {
		if !near(float64(c[i]), float64(want[i]), 1e-6) {
			t.Fatalf("ComplexGenerator: %v", c)
		}
	}
}

// firdS16Ref is esp-dsp dsps_fird_s16_ansi with shift 0.
func firdS16Ref(coeffs, delay []int16, pos, dPos *int, decim int, in, out []int16) {
	L := len(coeffs)
	ip := 0
	for i := range out {
		for j := 0; j < decim-*dPos; j++ {
			if *pos >= L {
				*pos = 0
			}
			delay[*pos] = in[ip]
			*pos++
			ip++
		}
		*dPos = 0
		acc := int64(0x7fff)
		c := L - 1
		for n := *pos; n < L; n++ {
			acc += int64(coeffs[c]) * int64(delay[n])
			c--
		}
		for n := 0; n < *pos; n++ {
			acc += int64(coeffs[c]) * int64(delay[n])
			c--
		}
		out[i] = int16(acc >> 15)
	}
}

func TestFIRDecimFilterInt16(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	coeffs := randInt16(r, 20)
	for i := range coeffs {
		coeffs[i] >>= 4
	}
	f := NewFIRDecimFilterInt16(coeffs, 3, 1, 0)
	in := randInt16(r, 3*40-1)
	got := make([]int16, 40)
	if n := f.Process(got, in[:3*10-1]); n != 10 {
		t.Fatalf("outputs: %d", n)
	}
	f.Process(got[10:], in[3*10-1:])
	want := make([]int16, 40)
	pos, dPos := 0, 1
	firdS16Ref(coeffs, make([]int16, 20), &pos, &dPos, 3, in, want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FIRDecimFilterInt16 i=%d: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestMultiRate(t *testing.T) {
	// Interpolate by 2 with a filter that repeats each sample.
	f := NewFIRMultiRateFloat32([]float32{1, 1}, 2, 1, 0)
	out := make([]float32, 8)
	if n := f.Process(out, []float32{1, 2, 3, 4}); n != 8 {
		t.Fatalf("outputs: %d", n)
	}
	if out[0] != 1 || out[1] != 1 || out[6] != 4 || out[7] != 4 {
		t.Errorf("FIRMultiRateFloat32: %v", out)
	}
	fi := NewFIRMultiRateInt16([]int16{32767, 32767}, 2, 1, 0, 0)
	o16 := make([]int16, 4)
	fi.Process(o16, []int16{1000, 2000})
	if o16[0] < 999 || o16[0] > 1001 || o16[3] < 1999 || o16[3] > 2001 {
		t.Errorf("FIRMultiRateInt16: %v", o16)
	}
	p := NewPhaseResamplerFloat32(2)
	po := make([]float32, 300)
	in := make([]float32, 100)
	for i := range in {
		in[i] = 1
	}
	n := p.Process(po, in)
	if n < 199 || n > 201 || po[n-1] != 1 {
		t.Errorf("PhaseResampler: n=%d last=%v", n, po[n-1])
	}
	rs := NewResamplerFloat32([]float32{0.5, 0.5, 0.5, 0.5}, 2, 1)
	ro := make([]float32, 200)
	if n := rs.Process(ro, in, 0); n < 99 || n > 101 {
		t.Errorf("Resampler outputs: %d", n)
	}
}

func TestStereoAndAnalysis(t *testing.T) {
	mono := NewLowPassFloat32(0.1, 0.7)
	st := NewBiquadStereoFloat32(mono)
	in := make([]float32, 64)
	for i := range in {
		in[i] = float32(i % 7)
	}
	left := make([]float32, 32)
	for i := range left {
		left[i] = in[2*i]
	}
	st.Process(in, in)
	mono.Process(left, left)
	for i := range left {
		if in[2*i] != left[i] {
			t.Fatalf("stereo left %d: %v %v", i, in[2*i], left[i])
		}
	}
	// A tone on bin 100 has no leakage out of the signal bins.
	x := make([]float32, 1024)
	ToneFloat32(x, 1, 100.0/1024, 0)
	if v := SNRFloat32(x, false); v < 60 {
		t.Errorf("SNRFloat32 of a clean tone: %v", v)
	}
	if v := SFDRFloat32(x, false); v < 60 {
		t.Errorf("SFDRFloat32 of a clean tone: %v", v)
	}
	c := make([]float32, 2048)
	NewComplexGenerator(1024, 0.1, 0).Float32(c)
	if v := SNRComplexFloat32(c); v < 40 {
		t.Errorf("SNRComplexFloat32: %v", v)
	}
	var buf bytes.Buffer
	View(&buf, x[:64], 32, 5, -1, 1, '*')
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 7 || !strings.Contains(buf.String(), "*") {
		t.Errorf("View:\n%s", buf.String())
	}
}
