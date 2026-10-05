package p256

import (
	"crypto/elliptic"
	"math/big"
	"math/rand"
	"testing"

	"runtime/memory/arena"
)

func rand32(r *rand.Rand, max *big.Int) (b [32]byte, v *big.Int) {
	v = new(big.Int).Rand(r, max)
	v.FillBytes(b[:])
	return b, v
}

func TestFieldOps(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	params := elliptic.P256().Params()
	for i := 0; i < 200; i++ {
		x, bx := rand32(r, bigP)
		var ys [32]byte
		if !YSquared(&x, &ys) {
			t.Fatal("YSquared rejected x below p")
		}
		want := new(big.Int).Exp(bx, big.NewInt(3), bigP)
		want.Sub(want, new(big.Int).Mul(bx, big.NewInt(3)))
		want.Add(want, params.B)
		want.Mod(want, bigP)
		if new(big.Int).SetBytes(ys[:]).Cmp(want) != 0 {
			t.Fatalf("YSquared(%x) = %x, want %x", x, ys, want)
		}
		square := big.Jacobi(want, bigP) >= 0
		if IsSquare(&ys) != square {
			t.Fatalf("IsSquare(%x) = %v, want %v", ys, !square, square)
		}
		var root [32]byte
		ok := Sqrt(&ys, &root)
		if ok != square {
			t.Fatalf("Sqrt ok = %v, want %v", ok, square)
		}
		if ok {
			rr := new(big.Int).SetBytes(root[:])
			if new(big.Int).Exp(rr, big.NewInt(2), bigP).Cmp(want) != 0 {
				t.Fatalf("Sqrt(%x) = %x is not a root", ys, root)
			}
			var neg [32]byte
			NegateY(&root, &neg)
			if new(big.Int).Add(rr, new(big.Int).SetBytes(neg[:])).Cmp(bigP) != 0 {
				t.Fatal("NegateY(y) + y != p")
			}
		}
	}
	var pb [32]byte
	bigP.FillBytes(pb[:])
	if BelowP(&pb) || YSquared(&pb, &pb) {
		t.Error("p is not below p")
	}
}

func TestPointOps(t *testing.T) {
	h, _ := arena.NewHeap(make([]byte, 8<<10))
	c := elliptic.P256()
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 20; i++ {
		k1, b1 := rand32(r, c.Params().N)
		k2, b2 := rand32(r, c.Params().N)
		x1, y1 := c.ScalarBaseMult(k1[:])
		x2, y2 := c.ScalarBaseMult(k2[:])
		var ax, ay, bx, by, sx, sy, mx, my [32]byte
		x1.FillBytes(ax[:])
		y1.FillBytes(ay[:])
		x2.FillBytes(bx[:])
		y2.FillBytes(by[:])
		if err := Add(&ax, &ay, &bx, &by, &sx, &sy); err != nil {
			t.Fatal(err)
		}
		wx, wy := c.Add(x1, y1, x2, y2)
		if new(big.Int).SetBytes(sx[:]).Cmp(wx) != 0 || new(big.Int).SetBytes(sy[:]).Cmp(wy) != 0 {
			t.Fatal("Add differs from crypto/elliptic")
		}
		if err := ScalarMult(h, &k2, &ax, &ay, &mx, &my); err != nil {
			t.Fatal(err)
		}
		wx, wy = c.ScalarMult(x1, y1, k2[:])
		if new(big.Int).SetBytes(mx[:]).Cmp(wx) != 0 || new(big.Int).SetBytes(my[:]).Cmp(wy) != 0 {
			t.Fatal("ScalarMult differs from crypto/elliptic")
		}
		// A point plus its inverse is the identity, and a point plus itself is its double.
		var ny [32]byte
		NegateY(&ay, &ny)
		if err := Add(&ax, &ay, &ax, &ny, &sx, &sy); err != ErrIdentity {
			t.Fatalf("P + -P: %v", err)
		}
		if err := Add(&ax, &ay, &ax, &ay, &sx, &sy); err != nil {
			t.Fatal(err)
		}
		wx, _ = c.Double(x1, y1)
		if new(big.Int).SetBytes(sx[:]).Cmp(wx) != 0 {
			t.Fatal("P + P differs from the double")
		}
		var s [32]byte
		AddModN(&k1, &k2, &s)
		ws := new(big.Int).Add(b1, b2)
		ws.Mod(ws, c.Params().N)
		if new(big.Int).SetBytes(s[:]).Cmp(ws) != 0 {
			t.Fatal("AddModN differs from math/big")
		}
	}
	if h.Stats().InUse != 0 {
		t.Errorf("memory in use: %d", h.Stats().InUse)
	}
	var bad [32]byte
	bad[31] = 1
	var ox, oy [32]byte
	if Add(&bad, &bad, &bad, &bad, &ox, &oy) != ErrPoint {
		t.Error("Add took a point that is not on the curve")
	}
}
