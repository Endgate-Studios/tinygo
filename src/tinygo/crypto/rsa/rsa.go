// Package rsa verifies RSA signatures with SHA-256 (RFC 8017): PKCS #1 v1.5
// for certificates and PSS for TLS 1.3. The inputs are public, so the code is
// not constant time. The working numbers come from an injected allocator.
package rsa

import (
	"crypto/subtle"

	"runtime/memory"
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
	w := memory.MakeSlice[uint32](mem, 6*limbs+2)
	if w == nil {
		return false
	}
	defer memory.FreeSlice(mem, w)
	n, s, r, acc, t := w[:limbs], w[limbs:2*limbs], w[2*limbs:3*limbs], w[3*limbs:4*limbs], w[4*limbs:6*limbs+2]
	fromBytes(n, k.N)
	fromBytes(s, sig)
	if !less(s, n) {
		return false
	}
	m := mont{n: n, inv: -inverse32(n[0]), t: t}
	// r = R^2 mod n, with R = 2^(32*limbs), by doubling.
	r[0] = 1
	for i := 0; i < 2*32*limbs; i++ {
		shiftAddMod(r, n)
	}
	// Montgomery form of s, then square and multiply for e.
	m.mul(acc, s, r) // acc = s * R
	copy(s, acc)
	top := 31
	for k.E>>top&1 == 0 {
		top--
	}
	for i := top - 1; i >= 0; i-- {
		m.mul(acc, acc, acc)
		if k.E>>i&1 == 1 {
			m.mul(acc, acc, s)
		}
	}
	// Out of Montgomery form: multiply by 1.
	clear(r)
	r[0] = 1
	m.mul(acc, acc, r)
	toBytes(em, acc)
	return true
}

// mont is Montgomery multiplication modulo n (Handbook of Applied
// Cryptography, algorithm 14.36), with 32-bit limbs.
type mont struct {
	n   []uint32
	inv uint32 // -n^-1 mod 2^32
	t   []uint32
}

// mul sets z = x * y / R mod n. z can be x or y.
func (m *mont) mul(z, x, y []uint32) {
	l := len(m.n)
	t := m.t[:l+2]
	clear(t)
	for i := 0; i < l; i++ {
		var c uint64
		for j := 0; j < l; j++ {
			v := uint64(t[j]) + uint64(x[i])*uint64(y[j]) + c
			t[j], c = uint32(v), v>>32
		}
		v := uint64(t[l]) + c
		t[l], t[l+1] = uint32(v), uint32(v>>32)
		u := t[0] * m.inv
		c = (uint64(t[0]) + uint64(u)*uint64(m.n[0])) >> 32
		for j := 1; j < l; j++ {
			v := uint64(t[j]) + uint64(u)*uint64(m.n[j]) + c
			t[j-1], c = uint32(v), v>>32
		}
		v = uint64(t[l]) + c
		t[l-1] = uint32(v)
		t[l] = t[l+1] + uint32(v>>32)
	}
	if t[l] != 0 || !less(t[:l], m.n) {
		sub(t[:l], m.n)
	}
	copy(z, t[:l])
}

// inverse32 gives x^-1 mod 2^32 for an odd x, by Newton iteration.
func inverse32(x uint32) uint32 {
	y := x
	for i := 0; i < 5; i++ {
		y *= 2 - x*y
	}
	return y
}

// shiftAddMod sets r = 2r mod n for r < n.
func shiftAddMod(r, n []uint32) {
	top := r[len(r)-1] >> 31
	for i := len(r) - 1; i > 0; i-- {
		r[i] = r[i]<<1 | r[i-1]>>31
	}
	r[0] <<= 1
	if top == 1 || !less(r, n) {
		sub(r, n)
	}
}

func less(a, b []uint32) bool {
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func sub(a, b []uint32) {
	var borrow uint64
	for i := range a {
		v := uint64(a[i]) - uint64(b[i]) - borrow
		a[i], borrow = uint32(v), (v>>32)&1
	}
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
