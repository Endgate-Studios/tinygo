package x25519

import (
	"math/big"
	"math/rand"
	"testing"
)

func elemBig(e *fe) *big.Int {
	b := new(big.Int)
	for i := 7; i >= 0; i-- {
		b.Lsh(b, 32).Or(b, big.NewInt(int64(e[i])))
	}
	return b
}

// TestAddSub compares feAdd and feSub with math/big, also at 0 and p - 1.
func TestAddSub(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	pb := elemBig(&p)
	edge := []fe{{}, {1}, p, {0xffffffff}}
	edge[2][0]-- // p - 1
	for i := 0; i < 5000; i++ {
		var a, b fe
		for _, e := range []*fe{&a, &b} {
			v := new(big.Int).Rand(r, pb)
			for k := range e {
				e[k] = uint32(new(big.Int).Rsh(v, uint(32*k)).Uint64())
			}
		}
		if i < len(edge)*len(edge) {
			a, b = edge[i%len(edge)], edge[i/len(edge)]
		}
		var s, d fe
		add(&s, &a, &b)
		sub(&d, &a, &b)
		ws := new(big.Int).Add(elemBig(&a), elemBig(&b))
		ws.Mod(ws, pb)
		wd := new(big.Int).Sub(elemBig(&a), elemBig(&b))
		wd.Mod(wd, pb)
		if elemBig(&s).Cmp(ws) != 0 || elemBig(&d).Cmp(wd) != 0 {
			t.Fatalf("%x %x: sum %x want %x, difference %x want %x", a, b, s, ws, d, wd)
		}
	}
}
