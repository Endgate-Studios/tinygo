// Package p384 verifies ECDSA signatures on the NIST P-384 curve (FIPS 186-4
// section 6.4.2, SEC 2 section 2.5.1). The inputs are public, so the code is
// not constant time. It does not allocate.
package p384

import "tinygo/crypto/internal/mpi"

// Size is the length of a coordinate and of a scalar in bytes.
const Size = 48

// nat is a number in 12 little-endian 32-bit limbs.
type nat [12]uint32

var (
	pNat = nat{0xffffffff, 0x00000000, 0x00000000, 0xffffffff, 0xfffffffe, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff}
	nNat = nat{0xccc52973, 0xecec196a, 0x48b0a77a, 0x581a0db2, 0xf4372ddf, 0xc7634d81, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff}
	bNat = nat{0xd3ec2aef, 0x2a85c8ed, 0x8a2ed19d, 0xc656398d, 0x5013875a, 0x0314088f, 0xfe814112, 0x181d9c6e, 0xe3f82d19, 0x988e056b, 0xe23ee7e4, 0xb3312fa7}
	gx   = nat{0x72760ab7, 0x3a545e38, 0xbf55296c, 0x5502f25d, 0x82542a38, 0x59f741e0, 0x8ba79b98, 0x6e1d3b62, 0xf320ad74, 0x8eb1c71e, 0xbe8b0537, 0xaa87ca22}
	gy   = nat{0x90ea0e5f, 0x7a431d7c, 0x1d7e819d, 0x0a60b1ce, 0xb5f0b8c0, 0xe9da3113, 0x289a147c, 0xf8f41dbd, 0x9292dc29, 0x5d9e98bf, 0x96262c6f, 0x3617de4a}
)

var (
	fp, fn field
	gen    point
)

func init() {
	fp.init(&pNat)
	fn.init(&nNat)
	gen = point{x: gx, y: gy, z: nat{1}}
}

func fromBytes(z *nat, b *[Size]byte) {
	for i := range z {
		o := Size - 4 - 4*i
		z[i] = uint32(b[o])<<24 | uint32(b[o+1])<<16 | uint32(b[o+2])<<8 | uint32(b[o+3])
	}
}

func less(a, b *nat) bool {
	for i := len(a) - 1; i >= 0; i-- {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// addNat sets z = x + y and gives the carry.
func addNat(z, x, y *nat) uint32 {
	var c uint64
	for i := range z {
		v := uint64(x[i]) + uint64(y[i]) + c
		z[i], c = uint32(v), v>>32
	}
	return uint32(c)
}

// subNat sets z = x - y and gives the borrow.
func subNat(z, x, y *nat) uint32 {
	var borrow uint64
	for i := range z {
		v := uint64(x[i]) - uint64(y[i]) - borrow
		z[i], borrow = uint32(v), (v>>32)&1
	}
	return uint32(borrow)
}

// field is arithmetic modulo an odd prime m. tinygo/crypto/internal/mpi
// multiplies, so the RSA accelerator does it on the ESP32-S3.
type field struct {
	m   nat
	mod mpi.Modulus
	rr  nat
	one nat
}

func (f *field) init(m *nat) {
	f.m = *m
	f.one = nat{1}
	f.mod.Init(f.m[:], f.rr[:])
}

// mul sets z = x * y mod m. z can be x or y.
func (f *field) mul(z, x, y *nat) { f.mod.Mul(z[:], x[:], y[:]) }

// invert sets z = x^(m-2), the inverse of x when x is not 0.
func (f *field) invert(z, x *nat) {
	e := f.m
	e[0] -= 2
	f.mod.Exp(z[:], x[:], e[:])
}

// point is a point in Jacobian coordinates. z == 0 is the point at
// infinity.
type point struct{ x, y, z nat }

// double uses dbl-2001-b for a = -3 (Explicit-Formulas Database,
// https://hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-3.html).
func (q *point) double(p *point) {
	var delta, gamma, beta, alpha, t1, t2, x3, y3, z3 nat
	fp.mul(&delta, &p.z, &p.z)
	fp.mul(&gamma, &p.y, &p.y)
	fp.mul(&beta, &p.x, &gamma)
	subP(&t1, &p.x, &delta)
	addP(&t2, &p.x, &delta)
	fp.mul(&alpha, &t1, &t2)
	addP(&t1, &alpha, &alpha)
	addP(&alpha, &alpha, &t1)
	addP(&t1, &p.y, &p.z)
	fp.mul(&z3, &t1, &t1)
	subP(&z3, &z3, &gamma)
	subP(&z3, &z3, &delta)
	addP(&t1, &beta, &beta)
	addP(&t1, &t1, &t1)
	addP(&t2, &t1, &t1)
	fp.mul(&x3, &alpha, &alpha)
	subP(&x3, &x3, &t2)
	subP(&t1, &t1, &x3)
	fp.mul(&y3, &alpha, &t1)
	fp.mul(&t2, &gamma, &gamma)
	addP(&t2, &t2, &t2)
	addP(&t2, &t2, &t2)
	addP(&t2, &t2, &t2)
	subP(&y3, &y3, &t2)
	q.x, q.y, q.z = x3, y3, z3
}

// add uses add-2007-bl, with the special cases of infinity and of equal
// points (Explicit-Formulas Database, as for double).
func (q *point) add(p1, p2 *point) {
	if p1.z == (nat{}) {
		*q = *p2
		return
	}
	if p2.z == (nat{}) {
		*q = *p1
		return
	}
	var z1z1, z2z2, u1, u2, s1, s2, h, i, j, r, v, t nat
	fp.mul(&z1z1, &p1.z, &p1.z)
	fp.mul(&z2z2, &p2.z, &p2.z)
	fp.mul(&u1, &p1.x, &z2z2)
	fp.mul(&u2, &p2.x, &z1z1)
	fp.mul(&t, &p2.z, &z2z2)
	fp.mul(&s1, &p1.y, &t)
	fp.mul(&t, &p1.z, &z1z1)
	fp.mul(&s2, &p2.y, &t)
	subP(&h, &u2, &u1)
	subP(&r, &s2, &s1)
	if h == (nat{}) {
		if r == (nat{}) {
			q.double(p1)
		} else {
			*q = point{}
		}
		return
	}
	addP(&r, &r, &r)
	addP(&i, &h, &h)
	fp.mul(&i, &i, &i)
	fp.mul(&j, &h, &i)
	fp.mul(&v, &u1, &i)
	var x3, y3, z3 nat
	fp.mul(&x3, &r, &r)
	subP(&x3, &x3, &j)
	subP(&x3, &x3, &v)
	subP(&x3, &x3, &v)
	subP(&t, &v, &x3)
	fp.mul(&y3, &r, &t)
	fp.mul(&t, &s1, &j)
	addP(&t, &t, &t)
	subP(&y3, &y3, &t)
	addP(&t, &p1.z, &p2.z)
	fp.mul(&t, &t, &t)
	subP(&t, &t, &z1z1)
	subP(&t, &t, &z2z2)
	fp.mul(&z3, &t, &h)
	q.x, q.y, q.z = x3, y3, z3
}

// onCurve reports whether y^2 = x^3 - 3x + b.
func onCurve(x, y *nat) bool {
	var lhs, rhs, t nat
	fp.mul(&lhs, y, y)
	fp.mul(&rhs, x, x)
	fp.mul(&rhs, &rhs, x)
	addP(&t, x, x)
	addP(&t, &t, x)
	subP(&rhs, &rhs, &t)
	addP(&rhs, &rhs, &bNat)
	return lhs == rhs
}

// affine reads (x, y) into q, with z = 1, and reports whether it is a point
// of the curve.
func (q *point) affine(x, y *[Size]byte) bool {
	fromBytes(&q.x, x)
	fromBytes(&q.y, y)
	if !less(&q.x, &pNat) || !less(&q.y, &pNat) {
		return false
	}
	q.z = nat{1}
	return onCurve(&q.x, &q.y)
}

// OnCurve reports whether (x, y) is a point of the curve.
func OnCurve(x, y *[Size]byte) bool {
	var q point
	return q.affine(x, y)
}

// VerifyECDSA reports whether (r, s) is a valid signature of hash with the
// public key (qx, qy). A hash longer than 48 bytes is cut to its first 48
// bytes (FIPS 186-4 section 6.4).
func VerifyECDSA(hash []byte, r, s, qx, qy *[Size]byte) bool {
	var rn, sn nat
	fromBytes(&rn, r)
	fromBytes(&sn, s)
	if rn == (nat{}) || sn == (nat{}) || !less(&rn, &nNat) || !less(&sn, &nNat) {
		return false
	}
	var q point
	if !q.affine(qx, qy) {
		return false
	}
	var eb [Size]byte
	if len(hash) >= Size {
		copy(eb[:], hash[:Size])
	} else {
		copy(eb[Size-len(hash):], hash)
	}
	var e nat
	fromBytes(&e, &eb)
	if !less(&e, &nNat) {
		subNat(&e, &e, &nNat)
	}
	var w, u1, u2 nat
	fn.invert(&w, &sn)
	fn.mul(&u1, &e, &w)
	fn.mul(&u2, &rn, &w)
	// u1 * G + u2 * Q with one chain of doublings and the width-4 NAF of
	// each scalar (Shamir's trick).
	var tg, tq table
	tg.oddMultiples(&gen)
	tq.oddMultiples(&q)
	var d1, d2 [nafLen]int8
	n1, n2 := naf(&d1, &u1), naf(&d2, &u2)
	var acc point
	for i := max(n1, n2) - 1; i >= 0; i-- {
		acc.double(&acc)
		acc.addDigit(&tg, d1[i])
		acc.addDigit(&tq, d2[i])
	}
	if acc.z == (nat{}) {
		return false
	}
	var zi, x nat
	fp.invert(&zi, &acc.z)
	fp.mul(&zi, &zi, &zi)
	fp.mul(&x, &acc.x, &zi)
	if !less(&x, &nNat) {
		subNat(&x, &x, &nNat)
	}
	return x == rn
}

// nafLen is the most digits of the NAF of a number below 2^384.
const nafLen = 8*Size + 1

// naf writes the width-4 NAF of k, the least significant digit first, and
// gives the number of digits. The digits are 0, 1, 3, 5 and 7 and their
// negatives (Hankerson, Menezes and Vanstone, Guide to Elliptic Curve
// Cryptography, algorithm 3.35).
func naf(d *[nafLen]int8, k *nat) int {
	var v [13]uint32
	copy(v[:], k[:])
	n := 0
	for v != ([13]uint32{}) {
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
		subP(&neg.y, &nat{}, &neg.y)
		q.add(q, &neg)
	}
}
