package p256

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"math/rand"
	"testing"

	"runtime/memory/arena"
)

// TestVerifyECDSA checks signatures of crypto/ecdsa and changed ones.
func TestVerifyECDSA(t *testing.T) {
	h, _ := arena.NewHeap(make([]byte, 8<<10))
	r := rand.New(rand.NewSource(5))
	for i := 0; i < 10; i++ {
		key, err := ecdsa.GenerateKey(elliptic.P256(), r)
		if err != nil {
			t.Fatal(err)
		}
		hash := make([]byte, []int{32, 48, 20}[i%3])
		r.Read(hash)
		sr, ss, err := ecdsa.Sign(r, key, hash)
		if err != nil {
			t.Fatal(err)
		}
		var rb, sb, qx, qy [32]byte
		sr.FillBytes(rb[:])
		ss.FillBytes(sb[:])
		key.X.FillBytes(qx[:])
		key.Y.FillBytes(qy[:])
		if !VerifyECDSA(h, hash, &rb, &sb, &qx, &qy) {
			t.Fatalf("signature %d of crypto/ecdsa does not verify", i)
		}
		bad := append([]byte(nil), hash...)
		bad[0] ^= 1
		if VerifyECDSA(h, bad, &rb, &sb, &qx, &qy) {
			t.Fatalf("signature %d verifies for another hash", i)
		}
		rb[31] ^= 1
		if VerifyECDSA(h, hash, &rb, &sb, &qx, &qy) {
			t.Fatalf("changed signature %d verifies", i)
		}
	}
	var zero [32]byte
	if VerifyECDSA(h, make([]byte, 32), &zero, &zero, &zero, &zero) {
		t.Error("zero signature verifies")
	}
	if h.Stats().InUse != 0 {
		t.Errorf("memory in use: %d", h.Stats().InUse)
	}
}
