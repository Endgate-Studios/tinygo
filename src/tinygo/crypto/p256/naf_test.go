package p256

import (
	"math/big"
	"math/rand"
	"testing"
)

// TestNAF checks that the digits give k back, and that each nonzero digit
// is odd and below 8, with 3 zeros after it.
func TestNAF(t *testing.T) {
	r := rand.New(rand.NewSource(8))
	max := new(big.Int).Lsh(big.NewInt(1), 256)
	for i := 0; i < 2000; i++ {
		kb := new(big.Int).Rand(r, max)
		if i == 0 {
			kb.Sub(max, big.NewInt(1))
		}
		var k scalar
		for j := range k {
			k[j] = uint32(new(big.Int).Rsh(kb, uint(32*j)).Uint64())
		}
		var d [nafLen]int8
		n := naf(&d, &k)
		sum := new(big.Int)
		for j := n - 1; j >= 0; j-- {
			sum.Lsh(sum, 1).Add(sum, big.NewInt(int64(d[j])))
			if d[j] != 0 {
				if d[j]%2 == 0 || d[j] > 7 || d[j] < -7 {
					t.Fatalf("digit %d", d[j])
				}
				for z := 1; z <= 3 && j+z < n; z++ {
					if d[j+z] != 0 {
						t.Fatalf("digits %d and %d too near", j, j+z)
					}
				}
			}
		}
		if sum.Cmp(kb) != 0 {
			t.Fatalf("%x gives %x", kb, sum)
		}
	}
}
