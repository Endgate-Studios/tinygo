package dsp

import "testing"

func TestVectorEx(t *testing.T) {
	a := []int16{32767, 9, 100, 9, -4}
	b := []int16{1, 50, 9}
	d := make([]int16, 6)
	// a at step 2 is 32767, 100, -4. esp-dsp truncates, so 32767+1 wraps.
	AddInt16Ex(d, a, b, 3, 2, 1, 2, 0)
	if d[0] != -32768 || d[2] != 150 || d[4] != 5 {
		t.Errorf("AddInt16Ex: %v", d)
	}
	AddInt16Ex(d, a, b, 3, 2, 1, 1, 1)
	if d[0] != 16384 || d[1] != 75 || d[2] != 2 {
		t.Errorf("AddInt16Ex shift: %v", d)
	}
	SubInt16Ex(d, a, b, 3, 2, 1, 1, 0)
	if d[0] != 32766 || d[1] != 50 || d[2] != -13 {
		t.Errorf("SubInt16Ex: %v", d)
	}
	MulInt16Ex(d, a, b, 3, 2, 1, 1, 1)
	if d[1] != 2500 || d[2] != -18 {
		t.Errorf("MulInt16Ex: %v", d)
	}
	d8 := make([]int8, 3)
	AddInt8Ex(d8, []int8{127, 1, 2}, []int8{1, 1, 1}, 3, 1, 1, 1, 0)
	if d8[0] != -128 || d8[2] != 3 {
		t.Errorf("AddInt8Ex: %v", d8)
	}
	MulInt8Ex(d8, []int8{10, 0, 20}, []int8{10, 20}, 2, 2, 1, 1, 2)
	if d8[0] != 25 || d8[1] != 100 {
		t.Errorf("MulInt8Ex: %v", d8)
	}
	MulConstInt16Ex(d, []int16{16384, 0, -32768}, 2, 16384, 2, 1)
	if d[0] != 8192 || d[1] != -16384 {
		t.Errorf("MulConstInt16Ex: %v", d)
	}
	f := make([]float32, 4)
	AddFloat32Ex(f, []float32{1, 0, 2}, []float32{10, 20}, 2, 2, 1, 2)
	if f[0] != 11 || f[2] != 22 {
		t.Errorf("AddFloat32Ex: %v", f)
	}
	MulConstFloat32Ex(f, []float32{1, 0, 2}, 2, 3, 2, 1)
	if f[0] != 3 || f[1] != 6 {
		t.Errorf("MulConstFloat32Ex: %v", f)
	}
	if v := DotInt16Q15([]int16{16384, 16384}, []int16{16384, 16384}, 0); v != 16384 {
		t.Errorf("DotInt16Q15: %v", v)
	}
}

func TestImage(t *testing.T) {
	// A 4 by 2 image, using every second column: values 1, 3 and 5, 7.
	in := Image[int16]{Data: []int16{1, 2, 3, 4, 5, 6, 7, 8}, StepX: 2, StepY: 1, Stride: 4}
	f := Image[int16]{Data: []int16{1, 1, 1, 1}, StepX: 1, StepY: 1, Stride: 2}
	if v := DotImageInt16(in, f, 2, 2, 0, 0); v != 16 {
		t.Errorf("DotImageInt16: %v", v)
	}
	if v := DotImageInt16(in, f, 2, 2, 2, 1); v != 8 {
		t.Errorf("DotImageInt16 offset and shift: %v", v)
	}
	u := Image[uint8]{Data: []uint8{200, 200}, StepX: 1, StepY: 1, Stride: 2}
	if v := DotImageUint8(u, u, 2, 1, 10, 0); v != uint8((80000+512)>>10) {
		t.Errorf("DotImageUint8: %v", v)
	}
	fi := Image[float32]{Data: []float32{1, 2, 3, 4}, StepX: 1, StepY: 1, Stride: 2}
	if v := DotImageFloat32(fi, fi, 2, 2, 1); v != 1*2+2*3+3*4+4*5 {
		t.Errorf("DotImageFloat32: %v", v)
	}
	src := Image[float32]{Data: []float32{1, 2, 3, 4, 5, 6, 7, 8, 9}, StepX: 1, StepY: 1, Stride: 3, SizeX: 3, SizeY: 3}
	k := Image[float32]{Data: []float32{0, 0, 0, 0, 1, 1, 0, 0, 0}, StepX: 1, StepY: 1, Stride: 3, SizeX: 3, SizeY: 3}
	out := Image[float32]{Data: make([]float32, 9), StepX: 1, StepY: 1, Stride: 3}
	ConvImageFloat32(out, src, k)
	want := []float32{3, 5, 3, 9, 11, 6, 15, 17, 9}
	for i := range want {
		if out.Data[i] != want[i] {
			t.Fatalf("ConvImageFloat32: %v", out.Data)
		}
	}
}

func TestResamplerInt16(t *testing.T) {
	r := NewResamplerInt16([]int16{16384, 16384, 16384, 16384}, 2, 1, 0)
	in := make([]int16, 100)
	for i := range in {
		in[i] = 1000
	}
	out := make([]int16, 200)
	if n := r.Process(out, in, 0); n < 99 || n > 101 {
		t.Errorf("ResamplerInt16 outputs: %d", n)
	}
}
