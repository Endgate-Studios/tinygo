// Package rsa verifies RSA signatures with SHA-256 (RFC 8017): PKCS #1 v1.5
// for certificates and PSS for TLS 1.3. The inputs are public, so the code is
// not constant time. The working numbers come from an injected allocator, and
// tinygo/crypto/internal/mpi multiplies, so the RSA accelerator does it on the
// ESP32-S3.
package rsa

import (
	"crypto/subtle"

	"runtime/memory"
	"tinygo/crypto/internal/mpi"
	"tinygo/crypto/sha256"
)

// Memory is where the working numbers of one verification come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// MaxBits is the largest modulus.
const MaxBits = 4096

// PublicKey is an RSA public key. N is big endian, without leading zeros.
type PublicKey struct {
	N []byte
	E uint32
}

// encrypt computes sig^e mod n (the RSA public operation) into em, with the
// length of n. It reports false when the input is not valid.
func encrypt(mem Memory, k *PublicKey, sig, em []byte) bool {
	nb := len(k.N)
	if nb == 0 || nb*8 > MaxBits+7 || len(sig) != nb || len(em) != nb || k.N[0] == 0 || k.N[nb-1]&1 == 0 || k.E < 3 || k.E&1 == 0 {
		return false
	}
	limbs := (nb + 3) / 4
	w := memory.MakeSlice[uint32](mem, 4*limbs)
	if w == nil {
		return false
	}
	defer memory.FreeSlice(mem, w)
	n, rr, s, acc := w[:limbs], w[limbs:2*limbs], w[2*limbs:3*limbs], w[3*limbs:]
	fromBytes(n, k.N)
	fromBytes(s, sig)
	var mod mpi.Modulus
	if !less(s, n) || !mod.Init(n, rr) {
		return false
	}
	copy(acc, s)
	top := 31
	for k.E>>top&1 == 0 {
		top--
	}
	for i := top - 1; i >= 0; i-- {
		mod.Mul(acc, acc, acc)
		if k.E>>i&1 == 1 {
			mod.Mul(acc, acc, s)
		}
	}
	toBytes(em, acc)
	return true
}

func less(a, b []uint32) bool {
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// fromBytes reads big-endian bytes into little-endian limbs.
func fromBytes(z []uint32, b []byte) {
	clear(z)
	for i := 0; i < len(b); i++ {
		z[i/4] |= uint32(b[len(b)-1-i]) << (8 * (i % 4))
	}
}

func toBytes(b []byte, z []uint32) {
	for i := 0; i < len(b); i++ {
		b[len(b)-1-i] = byte(z[i/4] >> (8 * (i % 4)))
	}
}

// digestInfo is the DER prefix of a SHA-256 DigestInfo (RFC 8017 section 9.2).
var digestInfo = []byte{0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20}

// VerifyPKCS1v15 reports whether sig is a PKCS #1 v1.5 signature of hash, a
// SHA-256 digest (RFC 8017 section 8.2.2).
func VerifyPKCS1v15(mem Memory, k *PublicKey, hash *[sha256.Size]byte, sig []byte) bool {
	em := memory.MakeSlice[byte](mem, len(k.N))
	if em == nil {
		return false
	}
	defer memory.FreeSlice(mem, em)
	if !encrypt(mem, k, sig, em) {
		return false
	}
	// EM = 0x00 || 0x01 || PS || 0x00 || DigestInfo || hash, with PS of 0xff.
	tLen := len(digestInfo) + sha256.Size
	if len(em) < tLen+11 || em[0] != 0 || em[1] != 1 {
		return false
	}
	ps := em[2 : len(em)-tLen-1]
	ok := 1
	for _, b := range ps {
		ok &= subtle.ConstantTimeByteEq(b, 0xff)
	}
	ok &= subtle.ConstantTimeByteEq(em[len(em)-tLen-1], 0)
	ok &= subtle.ConstantTimeCompare(em[len(em)-tLen:len(em)-sha256.Size], digestInfo)
	ok &= subtle.ConstantTimeCompare(em[len(em)-sha256.Size:], hash[:])
	return ok == 1
}

// VerifyPSS reports whether sig is a PSS signature of hash, a SHA-256
// digest, with MGF1-SHA256 and a salt of the hash length, as TLS 1.3 uses
// it (RFC 8017 section 9.1.2, RFC 8446 section 4.2.3).
func VerifyPSS(mem Memory, k *PublicKey, hash *[sha256.Size]byte, sig []byte) bool {
	em := memory.MakeSlice[byte](mem, len(k.N))
	if em == nil {
		return false
	}
	defer memory.FreeSlice(mem, em)
	if !encrypt(mem, k, sig, em) {
		return false
	}
	// emBits = modBits - 1. When modBits - 1 is a multiple of 8, EM is one
	// byte shorter and the first byte of the result must be 0.
	modBits := len(k.N)*8 - leadingZeros(k.N[0])
	emBits := modBits - 1
	emLen := (emBits + 7) / 8
	if emLen < len(em) {
		if em[0] != 0 {
			return false
		}
		em = em[1:]
	}
	const hLen, sLen = sha256.Size, sha256.Size
	if emLen < hLen+sLen+2 || em[emLen-1] != 0xbc {
		return false
	}
	db := em[:emLen-hLen-1]
	h := em[emLen-hLen-1 : emLen-1]
	topBits := byte(0xff >> (8*emLen - emBits))
	if db[0]&^topBits != 0 {
		return false
	}
	// DB = maskedDB XOR MGF1(H).
	var mask [sha256.Size]byte
	for counter, off := uint32(0), 0; off < len(db); counter++ {
		var d sha256.Digest
		d.Reset(sha256.Init, 0)
		d.Write(h)
		c := [4]byte{byte(counter >> 24), byte(counter >> 16), byte(counter >> 8), byte(counter)}
		d.Write(c[:])
		d.Sum(&mask)
		for i := 0; i < len(mask) && off < len(db); i++ {
			db[off] ^= mask[i]
			off++
		}
	}
	db[0] &= topBits
	// DB = PS of zeros || 0x01 || salt.
	ps := db[:len(db)-sLen-1]
	for _, b := range ps {
		if b != 0 {
			return false
		}
	}
	if db[len(db)-sLen-1] != 1 {
		return false
	}
	salt := db[len(db)-sLen:]
	// H' = SHA-256(8 zeros || hash || salt).
	var zeros [8]byte
	var d sha256.Digest
	d.Reset(sha256.Init, 0)
	d.Write(zeros[:])
	d.Write(hash[:])
	d.Write(salt)
	var h2 [sha256.Size]byte
	d.Sum(&h2)
	return subtle.ConstantTimeCompare(h2[:], h) == 1
}

func leadingZeros(b byte) int {
	n := 0
	for i := 7; i >= 0 && b>>i&1 == 0; i-- {
		n++
	}
	return n
}
