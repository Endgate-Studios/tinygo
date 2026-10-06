package p256

import "tinygo/crypto/internal/mpi"

// ECDSA verification (FIPS 186-4 section 6.4.2) for TLS and certificates.
// All inputs are public, so the arithmetic modulo n is simple and not
// constant time.

// scalar is a value modulo n in 8 little-endian 32-bit limbs.
type scalar [8]uint32

var (
	nScalar = scalarFromBytes(&n)
	modN    mpi.Modulus
	rrN     scalar
)

func init() { modN.Init(nScalar[:], rrN[:]) }

func scalarFromBytes(b *[32]byte) scalar {
	var s scalar
	for i := 0; i < 8; i++ {
		o := 28 - 4*i
		s[i] = uint32(b[o])<<24 | uint32(b[o+1])<<16 | uint32(b[o+2])<<8 | uint32(b[o+3])
	}
	return s
}

func (s *scalar) bytes(b *[32]byte) {
	for i := 0; i < 8; i++ {
		o := 28 - 4*i
		b[o], b[o+1], b[o+2], b[o+3] = byte(s[i]>>24), byte(s[i]>>16), byte(s[i]>>8), byte(s[i])
	}
}

// geq reports whether a >= b.
func geq(a, b *scalar) bool {
	for i := 7; i >= 0; i-- {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func sub(a, b *scalar) {
	var borrow uint64
	for i := range a {
		v := uint64(a[i]) - uint64(b[i]) - borrow
		a[i], borrow = uint32(v), (v>>32)&1
	}
}

// mulModN sets r = a * b mod n.
func mulModN(r, a, b *scalar) { modN.Mul(r[:], a[:], b[:]) }

// invModN sets r = a^(n-2) mod n, the inverse of a when a is not 0.
func invModN(r, a *scalar) {
	e := nScalar
	e[0] -= 2
	x := scalar{1}
	for i := 255; i >= 0; i-- {
		mulModN(&x, &x, &x)
		if e[i/32]>>(i%32)&1 == 1 {
			mulModN(&x, &x, a)
		}
	}
	*r = x
}

// VerifyECDSA reports whether (r, s) is a valid signature of hash with the
// public key (qx, qy). A hash longer than 32 bytes is cut to its first 32
// bytes, as FIPS 186-4 section 6.4 says. mem is not used.
func VerifyECDSA(mem Memory, hash []byte, r, s, qx, qy *[32]byte) bool {
	if !validScalar(r) || !validScalar(s) || !OnCurve(qx, qy) {
		return false
	}
	var eb [32]byte
	if len(hash) >= 32 {
		copy(eb[:], hash[:32])
	} else {
		copy(eb[32-len(hash):], hash)
	}
	e := scalarFromBytes(&eb)
	if geq(&e, &nScalar) {
		sub(&e, &nScalar)
	}
	rs, ss := scalarFromBytes(r), scalarFromBytes(s)
	var w, u1, u2 scalar
	invModN(&w, &ss)
	mulModN(&u1, &e, &w)
	mulModN(&u2, &rs, &w)
	// u1 * G + u2 * Q with one chain of doublings (Shamir's trick). The
	// inputs are public, so the branches on the bits are allowed.
	var q, both, acc point
	feFromBytes(&q.x, qx)
	feFromBytes(&q.y, qy)
	q.z = element{1}
	both.add(&gen, &q)
	acc.setIdentity()
	for i := 255; i >= 0; i-- {
		acc.double(&acc)
		b1, b2 := u1[i/32]>>(i%32)&1, u2[i/32]>>(i%32)&1
		switch {
		case b1 == 1 && b2 == 1:
			acc.add(&acc, &both)
		case b1 == 1:
			acc.add(&acc, &gen)
		case b2 == 1:
			acc.add(&acc, &q)
		}
	}
	var ex, ey element
	if !acc.affine(&ex, &ey) {
		return false
	}
	var x [32]byte
	feToBytes(&x, &ex)
	v := scalarFromBytes(&x)
	if geq(&v, &nScalar) {
		sub(&v, &nScalar)
	}
	return v == rs
}
