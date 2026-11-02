package field25519

import (
	"math/big"
	"math/rand"
	"testing"
)

func elemBig(e *Element) *big.Int {
	b := new(big.Int)
	for i := 7; i >= 0; i-- {
		b.Lsh(b, 32).Or(b, big.NewInt(int64(e[i])))
	}
	return b
}

func bigElem(v *big.Int) (e Element) {
	for k := range e {
		e[k] = uint32(new(big.Int).Rsh(v, uint(32*k)).Uint64())
	}
	return e
}

// randomPairs gives random elements below p, with the edge values 0, 1,
// p - 1 and 2^32 - 1 combined first.
func randomPairs(n int, f func(a, b *Element)) {
	r := rand.New(rand.NewSource(7))
	pb := elemBig(&p)
	edge := []Element{{}, {1}, p, {0xffffffff}}
	edge[2][0]-- // p - 1
	for i := 0; i < n; i++ {
		a, b := bigElem(new(big.Int).Rand(r, pb)), bigElem(new(big.Int).Rand(r, pb))
		if i < len(edge)*len(edge) {
			a, b = edge[i%len(edge)], edge[i/len(edge)]
		}
		f(&a, &b)
	}
}

// TestAddSub compares Add and Sub with math/big, also at 0 and p - 1.
func TestAddSub(t *testing.T) {
	pb := elemBig(&p)
	randomPairs(5000, func(a, b *Element) {
		var s, d Element
		s.Add(a, b)
		d.Sub(a, b)
		ws := new(big.Int).Add(elemBig(a), elemBig(b))
		ws.Mod(ws, pb)
		wd := new(big.Int).Sub(elemBig(a), elemBig(b))
		wd.Mod(wd, pb)
		if elemBig(&s).Cmp(ws) != 0 || elemBig(&d).Cmp(wd) != 0 {
			t.Fatalf("%x %x: sum %x want %x, difference %x want %x", *a, *b, s, ws, d, wd)
		}
	})
}

// TestMulInvert compares Mul, Neg, Invert, Pow22523 and Equal with math/big.
func TestMulInvert(t *testing.T) {
	pb := elemBig(&p)
	e22523 := new(big.Int).Rsh(new(big.Int).Sub(pb, big.NewInt(5)), 3)
	eInv := new(big.Int).Sub(pb, big.NewInt(2))
	randomPairs(200, func(a, b *Element) {
		var m, n, inv, pw Element
		m.Mul(a, b)
		n.Neg(a)
		inv.Invert(a)
		pw.Pow22523(a)
		ab, bb := elemBig(a), elemBig(b)
		wm := new(big.Int).Mul(ab, bb)
		wm.Mod(wm, pb)
		wn := new(big.Int).Neg(ab)
		wn.Mod(wn, pb)
		if elemBig(&m).Cmp(wm) != 0 || elemBig(&n).Cmp(wn) != 0 {
			t.Fatalf("%x %x: product %x want %x, negation %x want %x", *a, *b, m, wm, n, wn)
		}
		if w := new(big.Int).Exp(ab, eInv, pb); elemBig(&inv).Cmp(w) != 0 {
			t.Fatalf("%x: inverse %x want %x", *a, inv, w)
		}
		if w := new(big.Int).Exp(ab, e22523, pb); elemBig(&pw).Cmp(w) != 0 {
			t.Fatalf("%x: pow22523 %x want %x", *a, pw, w)
		}
		want := uint32(0)
		if ab.Cmp(bb) == 0 {
			want = 1
		}
		if a.Equal(b) != want || a.Equal(a) != 1 {
			t.Fatalf("%x %x: Equal %d, want %d", *a, *b, a.Equal(b), want)
		}
	})
}

// TestBytes checks SetBytes on values of p and above, which it reduces, and
// that Bytes is its inverse below p.
func TestBytes(t *testing.T) {
	pb := elemBig(&p)
	tests := []struct {
		name string
		v    *big.Int
	}{
		{"0", big.NewInt(0)},
		{"p - 1", new(big.Int).Sub(pb, big.NewInt(1))},
		{"p", pb},
		{"p + 1", new(big.Int).Add(pb, big.NewInt(1))},
		{"2^255 - 1", new(big.Int).Add(pb, big.NewInt(18))},
	}
	for _, tt := range tests {
		e := bigElem(tt.v)
		var b [32]byte
		e.Bytes(&b)
		b[31] |= 0x80 // ignored
		var got Element
		got.SetBytes(&b)
		want := new(big.Int).Mod(tt.v, pb)
		if elemBig(&got).Cmp(want) != 0 {
			t.Errorf("%s: SetBytes = %x, want %x", tt.name, got, want)
		}
	}
}

// TestSwapSelect checks Swap and Select for both bits.
func TestSwapSelect(t *testing.T) {
	a, b := Element{1, 2, 3, 4, 5, 6, 7, 8}, Element{9, 10, 11, 12, 13, 14, 15, 16}
	for _, bit := range []uint32{0, 1} {
		x, y := a, b
		x.Swap(&y, bit)
		if (bit == 0 && (x != a || y != b)) || (bit == 1 && (x != b || y != a)) {
			t.Errorf("Swap bit %d: %x %x", bit, x, y)
		}
		z := a
		z.Select(&b, bit)
		if (bit == 0 && z != a) || (bit == 1 && z != b) {
			t.Errorf("Select bit %d: %x", bit, z)
		}
	}
}

// TestExponents checks the exponents that mpi.Exp uses on the ESP32-S3.
func TestExponents(t *testing.T) {
	pb := elemBig(&p)
	if elemBig(&pMinus2).Cmp(new(big.Int).Sub(pb, big.NewInt(2))) != 0 {
		t.Error("p - 2")
	}
	if elemBig(&p58).Cmp(new(big.Int).Rsh(new(big.Int).Sub(pb, big.NewInt(5)), 3)) != 0 {
		t.Error("(p - 5) / 8")
	}
}
