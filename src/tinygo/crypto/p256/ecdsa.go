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
	// u1 * G + u2 * Q with one chain of doublings and the width-4 NAF of
	// each scalar (Shamir's trick). The inputs are public, so the branches
	// on the digits are allowed.
	var q point
	feFromBytes(&q.x, qx)
	feFromBytes(&q.y, qy)
	q.z = element{1}
	var tg, tq table
	tg.oddMultiples(&gen)
	tq.oddMultiples(&q)
	var d1, d2 [nafLen]int8
	n1, n2 := naf(&d1, &u1), naf(&d2, &u2)
	var acc point
	acc.setIdentity()
	for i := max(n1, n2) - 1; i >= 0; i-- {
		acc.double(&acc)
		acc.addDigit(&tg, d1[i])
		acc.addDigit(&tq, d2[i])
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

// nafLen is the most digits of the NAF of a number below 2^256.
const nafLen = 257

// naf writes the width-4 NAF of k, the least significant digit first, and
// gives the number of digits. The digits are 0, 1, 3, 5 and 7 and their
// negatives (Hankerson, Menezes and Vanstone, Guide to Elliptic Curve
// Cryptography, algorithm 3.35).
func naf(d *[nafLen]int8, k *scalar) int {
	var v [9]uint32
	copy(v[:], k[:])
	n := 0
	for v != ([9]uint32{}) {
		var digit int8
		if v[0]&1 == 1 {
			digit = int8(v[0] & 15)
			if digit >= 8 {
				digit -= 16
			}
			// v -= digit, so the next 3 bits are 0.
			if digit > 0 {
				c := uint32(digit)
				for i := range v {
					w := v[i]
					v[i] = w - c
					if w >= c {
						break
					}
					c = 1
				}
			} else {
				c := uint32(-digit)
				for i := range v {
					v[i] += c
					if v[i] >= c {
						break
					}
					c = 1
				}
			}
		}
		d[n] = digit
		n++
		for i := 0; i < len(v)-1; i++ {
			v[i] = v[i]>>1 | v[i+1]<<31
		}
		v[len(v)-1] >>= 1
	}
	return n
}

// table holds p, 3p, 5p and 7p for addDigit.
type table [4]point

// oddMultiples sets t to p, 3p, 5p and 7p.
func (t *table) oddMultiples(p *point) {
	var p2 point
	p2.double(p)
	t[0] = *p
	for i := 1; i < len(t); i++ {
		t[i].add(&t[i-1], &p2)
	}
}

// addDigit adds d times the point of t, for a digit of naf.
func (q *point) addDigit(t *table, d int8) {
	switch {
	case d > 0:
		q.add(q, &t[d/2])
	case d < 0:
		neg := t[-d/2]
		feSub(&neg.y, &element{}, &neg.y)
		q.add(q, &neg)
	}
}
