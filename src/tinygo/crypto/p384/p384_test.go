package p384

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"math/big"
	"testing"
)

func fixed(v *big.Int) *[Size]byte {
	var b [Size]byte
	v.FillBytes(b[:])
	return &b
}

func TestVerify(t *testing.T) {
	n := elliptic.P384().Params().N
	for i := 0; i < 40; i++ {
		k, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		qx, qy := fixed(k.X), fixed(k.Y)
		msg := []byte{byte(i), 1, 2, 3}
		h384 := sha512.Sum384(msg)
		h512 := sha512.Sum512(msg)
		for _, h := range [][]byte{h384[:], h512[:], h384[:32], make([]byte, 48)} {
			r, s, err := ecdsa.Sign(rand.Reader, k, h)
			if err != nil {
				t.Fatal(err)
			}
			rb, sb := fixed(r), fixed(s)
			if !VerifyECDSA(h, rb, sb, qx, qy) {
				t.Fatalf("key %d: a valid signature of a %d-byte hash fails", i, len(h))
			}
			if !VerifyECDSA(h, rb, fixed(new(big.Int).Sub(n, s)), qx, qy) {
				t.Fatal("n - s fails")
			}
			bad := append([]byte(nil), h...)
			bad[0] ^= 1
			if VerifyECDSA(bad, rb, sb, qx, qy) {
				t.Fatal("a changed hash passes")
			}
			r2 := *rb
			r2[Size-1] ^= 1
			if VerifyECDSA(h, &r2, sb, qx, qy) {
				t.Fatal("a changed r passes")
			}
			s2 := *sb
			s2[0] ^= 0x10
			if VerifyECDSA(h, rb, &s2, qx, qy) {
				t.Fatal("a changed s passes")
			}
		}
	}
}

func TestRejects(t *testing.T) {
	k, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	h := sha512.Sum384([]byte("x"))
	r, s, _ := ecdsa.Sign(rand.Reader, k, h[:])
	qx, qy := fixed(k.X), fixed(k.Y)
	params := elliptic.P384().Params()
	var zero [Size]byte
	if VerifyECDSA(h[:], &zero, fixed(s), qx, qy) || VerifyECDSA(h[:], fixed(r), &zero, qx, qy) {
		t.Fatal("a zero scalar passes")
	}
	if rn := new(big.Int).Add(r, params.N); rn.BitLen() <= 384 && VerifyECDSA(h[:], fixed(rn), fixed(s), qx, qy) {
		t.Fatal("r + n passes")
	}
	y2 := *qy
	y2[Size-1] ^= 1
	if OnCurve(qx, &y2) || VerifyECDSA(h[:], fixed(r), fixed(s), qx, &y2) {
		t.Fatal("a point that is not on the curve passes")
	}
	if !OnCurve(qx, qy) || !OnCurve(fixed(params.Gx), fixed(params.Gy)) {
		t.Fatal("a point of the curve fails")
	}
	// The key G: then u1 * G and u2 * Q add a point to itself or to its negative.
	g := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P384(), X: params.Gx, Y: params.Gy}, D: big.NewInt(1)}
	for i := 0; i < 20; i++ {
		hh := sha512.Sum384([]byte{byte(i)})
		r, s, _ := ecdsa.Sign(rand.Reader, g, hh[:])
		if !VerifyECDSA(hh[:], fixed(r), fixed(s), fixed(params.Gx), fixed(params.Gy)) {
			t.Fatal("a signature with the key G fails")
		}
	}
}
