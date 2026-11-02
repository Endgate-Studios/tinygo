package mpi

import (
	"math/big"
	"math/rand"
	"testing"
)

func toBig(w []uint32) *big.Int {
	b := new(big.Int)
	for i := len(w) - 1; i >= 0; i-- {
		b.Lsh(b, 32)
		b.Or(b, big.NewInt(int64(w[i])))
	}
	return b
}

func fromBig(w []uint32, b *big.Int) {
	t := new(big.Int).Set(b)
	mask := big.NewInt(0xffffffff)
	for i := range w {
		w[i] = uint32(new(big.Int).And(t, mask).Uint64())
		t.Rsh(t, 32)
	}
}

func TestMul(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, n := range []int{1, 2, 8, 12, 64, 128} {
		for k := 0; k < 20; k++ {
			m := make([]uint32, n)
			for i := range m {
				m[i] = r.Uint32()
			}
			m[0] |= 1
			if k == 0 {
				for i := range m {
					m[i] = 0xffffffff
				}
			}
			if m[n-1] == 0 {
				m[n-1] = 1
			}
			var mod Modulus
			if !mod.Init(m, make([]uint32, n)) {
				t.Fatalf("%d words: Init failed", n)
			}
			mb := toBig(m)
			for j := 0; j < 10; j++ {
				x, y, z := make([]uint32, n), make([]uint32, n), make([]uint32, n)
				xb := new(big.Int).Rand(r, mb)
				yb := new(big.Int).Rand(r, mb)
				if j == 0 {
					xb.Sub(mb, big.NewInt(1))
					yb.Set(xb)
				}
				fromBig(x, xb)
				fromBig(y, yb)
				mod.Mul(z, x, y)
				want := new(big.Int).Mul(xb, yb)
				want.Mod(want, mb)
				if toBig(z).Cmp(want) != 0 {
					t.Fatalf("%d words: %x * %x mod %x = %x, want %x", n, xb, yb, mb, toBig(z), want)
				}
				mod.Mul(x, x, x)
				want.Mul(xb, xb).Mod(want, mb)
				if toBig(x).Cmp(want) != 0 {
					t.Fatalf("%d words: square in place", n)
				}
			}
		}
	}
}

func TestInitRejects(t *testing.T) {
	var mod Modulus
	for _, m := range [][]uint32{nil, {2}, {1}, {3, 0}} {
		if mod.Init(m, make([]uint32, len(m))) {
			t.Errorf("%x accepted", m)
		}
	}
}

func TestExp(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for _, n := range []int{1, 8, 12, 64} {
		for k := 0; k < 5; k++ {
			m := make([]uint32, n)
			for i := range m {
				m[i] = r.Uint32()
			}
			m[0] |= 1
			m[n-1] |= 1
			var mod Modulus
			mod.Init(m, make([]uint32, n))
			mb := toBig(m)
			x, e, z := make([]uint32, n), make([]uint32, n), make([]uint32, n)
			xb := new(big.Int).Rand(r, mb)
			eb := new(big.Int).Rand(r, mb)
			fromBig(x, xb)
			fromBig(e, eb)
			mod.Exp(z, x, e)
			if want := new(big.Int).Exp(xb, eb, mb); toBig(z).Cmp(want) != 0 {
				t.Fatalf("%d words: %x^%x mod %x = %x, want %x", n, xb, eb, mb, toBig(z), want)
			}
		}
	}
}

func TestAddSub(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, n := range []int{1, 2, 8, 96, 128} {
		for k := 0; k < 10; k++ {
			m := make([]uint32, n)
			for i := range m {
				m[i] = r.Uint32()
			}
			m[0] |= 1
			if k == 0 {
				for i := range m {
					m[i] = 0xffffffff
				}
			}
			if k == 1 && n > 1 {
				// Top bit clear, so a sum below 2^(32n) can still reach m.
				m[n-1] = 1
			}
			if m[n-1] == 0 {
				m[n-1] = 1
			}
			var mod Modulus
			if !mod.Init(m, make([]uint32, n)) {
				t.Fatalf("%d words: Init failed", n)
			}
			mb := toBig(m)
			max := new(big.Int).Sub(mb, big.NewInt(1))
			for j := 0; j < 20; j++ {
				xb := new(big.Int).Rand(r, mb)
				yb := new(big.Int).Rand(r, mb)
				switch j {
				case 0:
					xb.Set(max)
					yb.Set(max)
				case 1:
					xb.SetInt64(0)
					yb.Set(max)
				case 2:
					xb.SetInt64(0)
					yb.SetInt64(0)
				case 3:
					xb.Set(max)
					yb.SetInt64(1)
				}
				x, y, z := make([]uint32, n), make([]uint32, n), make([]uint32, n)
				fromBig(x, xb)
				fromBig(y, yb)
				mod.Add(z, x, y)
				want := new(big.Int).Add(xb, yb)
				want.Mod(want, mb)
				if toBig(z).Cmp(want) != 0 {
					t.Fatalf("%d words: %x + %x mod %x = %x, want %x", n, xb, yb, mb, toBig(z), want)
				}
				mod.Sub(z, x, y)
				want.Sub(xb, yb).Mod(want, mb)
				if toBig(z).Cmp(want) != 0 {
					t.Fatalf("%d words: %x - %x mod %x = %x, want %x", n, xb, yb, mb, toBig(z), want)
				}
				// In place: x = x + y, then x = x - y gives x back.
				mod.Add(x, x, y)
				mod.Sub(x, x, y)
				if toBig(x).Cmp(xb) != 0 {
					t.Fatalf("%d words: add then subtract in place gives %x, want %x", n, toBig(x), xb)
				}
			}
		}
	}
}
