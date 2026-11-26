package dsp

import (
	"math"
	"math/rand"
	"testing"
	"unsafe"
)

func randInt16(r *rand.Rand, n int) []int16 {
	s := make([]int16, n)
	for i := range s {
		s[i] = int16(r.Uint32())
	}
	return s
}

func TestDot(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	a, b := randInt16(r, 300), randInt16(r, 300)
	a[0], b[0] = -32768, -32768
	for _, n := range []int{0, 1, 8, 9, 257} {
		for off := 0; off < 8; off++ {
			var want int64
			for i := 0; i < n; i++ {
				want += int64(a[off+i]) * int64(b[i])
			}
			if got := DotInt16(a[off:off+n], b[:n]); got != want {
				t.Errorf("DotInt16 n=%d off=%d: got %d, want %d", n, off, got, want)
			}
		}
	}
	a8 := []int8{-128, 127, 5}
	b8 := []int8{-128, -128, 3}
	if got := DotInt8(a8, b8); got != 16384-16256+15 {
		t.Errorf("DotInt8: got %d", got)
	}
}

func TestAddInt16(t *testing.T) {
	dst := make([]int16, 3)
	AddInt16(dst, []int16{32767, -32768, 5}, []int16{1, -1, 6})
	if dst[0] != 32767 || dst[1] != -32768 || dst[2] != 11 {
		t.Errorf("AddInt16: got %v", dst)
	}
}

func TestMulInt16(t *testing.T) {
	dst := make([]int16, 3)
	MulInt16(dst, []int16{-32768, 32767, -1000}, []int16{-32768, 32767, 1000}, 15)
	if dst[0] != -32768 || dst[1] != 32766 || dst[2] != -31 {
		t.Errorf("MulInt16: got %v", dst)
	}
}

func TestFIRInt16(t *testing.T) {
	x := []int16{1, 2, 3, 4, 5}
	h := []int16{1, 0, -1}
	dst := make([]int16, 3)
	FIRInt16(dst, x, h, 0)
	if dst[0] != -2 || dst[1] != -2 || dst[2] != -2 {
		t.Errorf("FIRInt16: got %v", dst)
	}
	FIRInt16(dst[:1], []int16{32767, 32767}, []int16{32767, 32767}, 0)
	if dst[0] != 32767 {
		t.Errorf("FIRInt16 saturation: got %d", dst[0])
	}
}

func TestLengthMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic")
		}
	}()
	DotInt16(make([]int16, 2), make([]int16, 3))
}

func TestMatMulInt16(t *testing.T) {
	a := []int16{1, 2, 3, 4, 5, 6}    // 2x3
	b := []int16{7, 8, 9, 10, 11, 12} // 3x2
	dst := make([]int16, 4)
	MatMulInt16(dst, a, b, 2, 3, 2, 0)
	want := []int16{58, 64, 139, 154}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("MatMulInt16: got %v, want %v", dst, want)
		}
	}
	MatMulInt16(dst[:1], []int16{32767, 32767}, []int16{32767, 32767}, 1, 2, 1, 0)
	if dst[0] != 32767 {
		t.Errorf("MatMulInt16 saturation: got %d", dst[0])
	}
}

func TestFFTInt16(t *testing.T) {
	const n = 64
	f := NewFFTInt16(n)
	x := AlignedInt16(2 * n)
	for i := 0; i < n; i++ {
		x[2*i] = int16(16000 * math.Cos(2*math.Pi*5*float64(i)/n))
	}
	f.Transform(x)
	for i := 0; i < n; i++ {
		re, im := int(x[2*i]), int(x[2*i+1])
		want := 0
		if i == 5 || i == n-5 {
			want = 8000
		}
		if re < want-4 || re > want+4 || im < -4 || im > 4 {
			t.Errorf("bin %d: got %d%+di, want %d", i, re, im, want)
		}
	}
}

func TestAligned(t *testing.T) {
	for n := 0; n < 40; n++ {
		a := AlignedInt16(n)
		b := AlignedInt8(n)
		if len(a) != n || len(b) != n {
			t.Fatalf("n=%d: lengths %d %d", n, len(a), len(b))
		}
		if n > 0 && (uintptr(unsafe.Pointer(&a[0]))%16 != 0 || uintptr(unsafe.Pointer(&b[0]))%16 != 0) {
			t.Errorf("n=%d: not aligned", n)
		}
	}
}
