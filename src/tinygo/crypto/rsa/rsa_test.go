package rsa

import (
	"crypto"
	"crypto/rand"
	stdrsa "crypto/rsa"
	"crypto/sha256"
	"testing"

	"runtime/memory/arena"
)

func TestVerify(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 16<<10))
	for _, bits := range []int{1024, 2047, 2048, 3072, 4096} {
		key, err := stdrsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			t.Fatal(err)
		}
		pub := &PublicKey{N: key.N.Bytes(), E: uint32(key.E)}
		hash := sha256.Sum256([]byte("message"))
		v15, _ := stdrsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		pss, _ := stdrsa.SignPSS(rand.Reader, key, crypto.SHA256, hash[:], &stdrsa.PSSOptions{SaltLength: stdrsa.PSSSaltLengthEqualsHash})
		h := [32]byte(hash)
		if !VerifyPKCS1v15(mem, pub, &h, v15) {
			t.Errorf("%d bits: PKCS #1 v1.5 signature does not verify", bits)
		}
		if !VerifyPSS(mem, pub, &h, pss) {
			t.Errorf("%d bits: PSS signature does not verify", bits)
		}
		if VerifyPSS(mem, pub, &h, v15) || VerifyPKCS1v15(mem, pub, &h, pss) {
			t.Errorf("%d bits: a signature verifies with the other scheme", bits)
		}
		other := h
		other[0] ^= 1
		if VerifyPKCS1v15(mem, pub, &other, v15) || VerifyPSS(mem, pub, &other, pss) {
			t.Errorf("%d bits: a signature verifies for another hash", bits)
		}
		v15[len(v15)-1] ^= 1
		if VerifyPKCS1v15(mem, pub, &h, v15) {
			t.Errorf("%d bits: a changed signature verifies", bits)
		}
	}
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use: %d", mem.Stats().InUse)
	}
}
