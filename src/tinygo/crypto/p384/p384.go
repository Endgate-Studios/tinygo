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

func (f *field) add(z, x, y *nat) {
	if addNat(z, x, y) != 0 || !less(z, &f.m) {
		subNat(z, z, &f.m)
	}
}

func (f *field) sub(z, x, y *nat) {
	if subNat(z, x, y) != 0 {
		addNat(z, z, &f.m)
	}
}

// mul sets z = x * y mod m. z can be x or y.
func (f *field) mul(z, x, y *nat) { f.mod.Mul(z[:], x[:], y[:]) }

// invert sets z = x^(m-2), the inverse of x when x is not 0.
func (f *field) invert(z, x *nat) {
	e := f.m
	e[0] -= 2
	acc := f.one
	for i := 383; i >= 0; i-- {
		f.mul(&acc, &acc, &acc)
		if e[i/32]>>(i%32)&1 == 1 {
			f.mul(&acc, &acc, x)
		}
	}
	*z = acc
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
	fp.sub(&t1, &p.x, &delta)
	fp.add(&t2, &p.x, &delta)
	fp.mul(&alpha, &t1, &t2)
	fp.add(&t1, &alpha, &alpha)
	fp.add(&alpha, &alpha, &t1)
	fp.add(&t1, &p.y, &p.z)
	fp.mul(&z3, &t1, &t1)
	fp.sub(&z3, &z3, &gamma)
	fp.sub(&z3, &z3, &delta)
	fp.add(&t1, &beta, &beta)
	fp.add(&t1, &t1, &t1)
	fp.add(&t2, &t1, &t1)
	fp.mul(&x3, &alpha, &alpha)
	fp.sub(&x3, &x3, &t2)
	fp.sub(&t1, &t1, &x3)
	fp.mul(&y3, &alpha, &t1)
	fp.mul(&t2, &gamma, &gamma)
	fp.add(&t2, &t2, &t2)
	fp.add(&t2, &t2, &t2)
	fp.add(&t2, &t2, &t2)
	fp.sub(&y3, &y3, &t2)
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
	fp.sub(&h, &u2, &u1)
	fp.sub(&r, &s2, &s1)
	if h == (nat{}) {
		if r == (nat{}) {
			q.double(p1)
		} else {
			*q = point{}
		}
		return
	}
	fp.add(&r, &r, &r)
	fp.add(&i, &h, &h)
	fp.mul(&i, &i, &i)
	fp.mul(&j, &h, &i)
	fp.mul(&v, &u1, &i)
	var x3, y3, z3 nat
	fp.mul(&x3, &r, &r)
	fp.sub(&x3, &x3, &j)
	fp.sub(&x3, &x3, &v)
	fp.sub(&x3, &x3, &v)
	fp.sub(&t, &v, &x3)
	fp.mul(&y3, &r, &t)
	fp.mul(&t, &s1, &j)
	fp.add(&t, &t, &t)
	fp.sub(&y3, &y3, &t)
	fp.add(&t, &p1.z, &p2.z)
	fp.mul(&t, &t, &t)
	fp.sub(&t, &t, &z1z1)
	fp.sub(&t, &t, &z2z2)
	fp.mul(&z3, &t, &h)
	q.x, q.y, q.z = x3, y3, z3
}

// onCurve reports whether y^2 = x^3 - 3x + b.
func onCurve(x, y *nat) bool {
	var lhs, rhs, t nat
	fp.mul(&lhs, y, y)
	fp.mul(&rhs, x, x)
	fp.mul(&rhs, &rhs, x)
	fp.add(&t, x, x)
	fp.add(&t, &t, x)
	fp.sub(&rhs, &rhs, &t)
	fp.add(&rhs, &rhs, &bNat)
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
	// u1 * G + u2 * Q with one chain of doublings (Shamir's trick).
	var both, acc point
	both.add(&gen, &q)
	for i := 383; i >= 0; i-- {
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
