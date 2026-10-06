package x25519

import (
	"crypto/ecdh"
	"encoding/hex"
	"math/rand"
	"testing"
)

func h32(t *testing.T, s string) [32]byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatal(s)
	}
	return [32]byte(b)
}

// TestRFC7748 uses the vectors of RFC 7748 sections 5.2 and 6.1. The output
// of the second vector is the value of crypto/ecdh.
func TestRFC7748(t *testing.T) {
	for _, tt := range []struct{ k, u, out string }{
		{"a546e36bf0527c9d3b16154b82465edd62144c0ac1fc5a18506a2244ba449ac4", "e6db6867583030db3594c1a424b15f7c726624ec26b3353b10a903a6d0ab1c4c",
			"c3da55379de9c6908e94ea4df28d084f32eccf03491c71f754b4075577a28552"},
		{"4b66e9d4d1b4673c5ad22691957d6af5c11b6421e0ea01d42ca4169e7918ba0d", "e5210f12786811d3f4b7959d0538ae2c31dbe7106fc03c3efc4cd549c715a493",
			"95cbde9476e8907d7aade45cb4b873f88b595a68799fa152e6f8f7647aac7957"},
	} {
		k, u := h32(t, tt.k), h32(t, tt.u)
		var out [32]byte
		X25519(&out, &k, &u)
		if hex.EncodeToString(out[:]) != tt.out {
			t.Errorf("X25519 = %x, want %s", out, tt.out)
		}
	}
	// Iterations: k = u = 9, then u = old k, k = result.
	k, u := Basepoint, Basepoint
	for i := 1; i <= 1000; i++ {
		var r [32]byte
		X25519(&r, &k, &u)
		u, k = k, r
		if i == 1 && hex.EncodeToString(k[:]) != "422c8e7a6227d7bca1350b3e2bb7279f7897b87bb6854b783c60e80311ae3079" {
			t.Fatalf("1 iteration: %x", k)
		}
	}
	if hex.EncodeToString(k[:]) != "684cf59ba83309552800ef566f2f4d3c1c3887c49360e3875f2eb94d99532c51" {
		t.Errorf("1000 iterations: %x", k)
	}
	// Diffie-Hellman of section 6.1.
	alice := h32(t, "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a")
	bob := h32(t, "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb")
	var ap, bp, s1, s2 [32]byte
	X25519(&ap, &alice, &Basepoint)
	X25519(&bp, &bob, &Basepoint)
	if hex.EncodeToString(ap[:]) != "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a" ||
		hex.EncodeToString(bp[:]) != "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f" {
		t.Errorf("public keys %x %x", ap, bp)
	}
	X25519(&s1, &alice, &bp)
	X25519(&s2, &bob, &ap)
	if s1 != s2 || hex.EncodeToString(s1[:]) != "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742" {
		t.Errorf("shared %x %x", s1, s2)
	}
}

// TestECDH compares random scalars and points with crypto/ecdh.
func TestECDH(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for i := 0; i < 50; i++ {
		var k, kb [32]byte
		r.Read(k[:])
		r.Read(kb[:])
		var u [32]byte
		X25519(&u, &kb, &Basepoint) // a valid point
		priv, _ := ecdh.X25519().NewPrivateKey(k[:])
		pub, err := ecdh.X25519().NewPublicKey(u[:])
		if err != nil {
			t.Fatal(err)
		}
		want, err := priv.ECDH(pub)
		if err != nil {
			t.Fatal(err)
		}
		var got [32]byte
		X25519(&got, &k, &u)
		if hex.EncodeToString(got[:]) != hex.EncodeToString(want) {
			t.Fatalf("scalar %x: %x, want %x", k, got, want)
		}
	}
}

// TestNonCanonical uses u-coordinates of p and more and with the top bit set,
// which RFC 7748 section 5 reduces modulo p.
func TestNonCanonical(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	var us [][32]byte
	for _, low := range []byte{0xed, 0xee, 0xf0, 0xff} {
		var u [32]byte
		for i := range u {
			u[i] = 0xff
		}
		u[0], u[31] = low, 0x7f
		us = append(us, u)
		u[31] = 0xff
		us = append(us, u)
	}
	for i := 0; i < 10; i++ {
		var u [32]byte
		r.Read(u[:])
		u[31] |= 0x80
		us = append(us, u)
	}
	for _, u := range us {
		var k [32]byte
		r.Read(k[:])
		priv, _ := ecdh.X25519().NewPrivateKey(k[:])
		pub, _ := ecdh.X25519().NewPublicKey(u[:])
		var got [32]byte
		X25519(&got, &k, &u)
		want, err := priv.ECDH(pub)
		if err != nil {
			want = make([]byte, 32) // a point of low order gives zero
		}
		if hex.EncodeToString(got[:]) != hex.EncodeToString(want) {
			t.Errorf("u %x: %x, want %x", u, got, want)
		}
	}
}
