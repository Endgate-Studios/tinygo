// Package x25519 is the X25519 function of RFC 7748, constant time. The
// field multiplication is tinygo/crypto/internal/mpi, so the RSA accelerator
// does it on the ESP32-S3. It does not allocate.
package x25519

import "tinygo/crypto/internal/mpi"

// fe is an element of GF(2^255 - 19) in 8 little-endian 32-bit words,
// always below p.
type fe [8]uint32

var (
	p    = fe{0xffffffed, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0x7fffffff}
	a24  = fe{121665}
	modP mpi.Modulus
	rr   fe
)

func init() { modP.Init(p[:], rr[:]) }

// Basepoint is the u-coordinate 9 (RFC 7748 section 4.1).
var Basepoint = [32]byte{9}

func mul(z, a, b *fe) { modP.Mul(z[:], a[:], b[:]) }

// reduce subtracts p from z when z is at least p, with a mask.
func reduce(z *fe) {
	var d fe
	var borrow uint64
	for i := range d {
		v := uint64(z[i]) - uint64(p[i]) - borrow
		d[i], borrow = uint32(v), (v>>32)&1
	}
	keep := -uint32(borrow) // all ones when z < p
	for i := range z {
		z[i] = z[i]&keep | d[i]&^keep
	}
}

// swap exchanges a and b when bit is 1, in constant time.
func swap(a, b *fe, bit uint32) {
	mask := -bit
	for i := range a {
		t := mask & (a[i] ^ b[i])
		a[i] ^= t
		b[i] ^= t
	}
}

func square(z, a *fe, n int) {
	mul(z, a, a)
	for i := 1; i < n; i++ {
		mul(z, z, z)
	}
}

// invert sets z = a^(p-2). Without the hardware, the addition chain of ref10
// fe_invert needs fewer multiplications than Exp.
func invert(z, a *fe) {
	if mpi.HardwareExp {
		e := p
		e[0] -= 2
		modP.Exp(z[:], a[:], e[:])
		return
	}
	var z2, z9, z11, z5, z10, z20, z50, z100, t fe
	mul(&z2, a, a)
	square(&t, &z2, 2)
	mul(&z9, &t, a)
	mul(&z11, &z9, &z2)
	mul(&t, &z11, &z11)
	mul(&z5, &t, &z9) // 2^5 - 1
	square(&t, &z5, 5)
	mul(&z10, &t, &z5) // 2^10 - 1
	square(&t, &z10, 10)
	mul(&z20, &t, &z10) // 2^20 - 1
	square(&t, &z20, 20)
	mul(&t, &t, &z20) // 2^40 - 1
	square(&t, &t, 10)
	mul(&z50, &t, &z10) // 2^50 - 1
	square(&t, &z50, 50)
	mul(&z100, &t, &z50) // 2^100 - 1
	square(&t, &z100, 100)
	mul(&t, &t, &z100) // 2^200 - 1
	square(&t, &t, 50)
	mul(&t, &t, &z50) // 2^250 - 1
	square(&t, &t, 5)
	mul(z, &t, &z11) // 2^255 - 21
	z2, z9, z11, z5, z10, z20, z50, z100, t = fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}
}

// X25519 sets out to the scalar times the point with u-coordinate point
// (RFC 7748 section 5).
func X25519(out, scalar, point *[32]byte) {
	k := *scalar
	k[31] = k[31]&127 | 64
	k[0] &= 248
	var x1, x2, z2, x3, z3, a, aa, b, bb, e, c, d, da, cb fe
	for i := range x1 {
		x1[i] = uint32(point[4*i]) | uint32(point[4*i+1])<<8 | uint32(point[4*i+2])<<16 | uint32(point[4*i+3])<<24
	}
	x1[7] &= 0x7fffffff
	reduce(&x1)
	x2[0], x3, z3[0] = 1, x1, 1
	var swapped uint32
	for t := 254; t >= 0; t-- {
		bit := uint32(k[t>>3]>>(t&7)) & 1
		swapped ^= bit
		swap(&x2, &x3, swapped)
		swap(&z2, &z3, swapped)
		swapped = bit
		add(&a, &x2, &z2)
		mul(&aa, &a, &a)
		sub(&b, &x2, &z2)
		mul(&bb, &b, &b)
		sub(&e, &aa, &bb)
		add(&c, &x3, &z3)
		sub(&d, &x3, &z3)
		mul(&da, &d, &a)
		mul(&cb, &c, &b)
		add(&x3, &da, &cb)
		mul(&x3, &x3, &x3)
		sub(&z3, &da, &cb)
		mul(&z3, &z3, &z3)
		mul(&z3, &z3, &x1)
		mul(&x2, &aa, &bb)
		mul(&z2, &a24, &e)
		add(&z2, &z2, &aa)
		mul(&z2, &z2, &e)
	}
	swap(&x2, &x3, swapped)
	swap(&z2, &z3, swapped)
	invert(&z2, &z2)
	mul(&x2, &x2, &z2)
	for i, w := range x2 {
		out[4*i], out[4*i+1], out[4*i+2], out[4*i+3] = byte(w), byte(w>>8), byte(w>>16), byte(w>>24)
	}
	clear(k[:])
	x1, x2, z2, x3, z3, a, aa, b, bb, e, c, d, da, cb = fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}
}
