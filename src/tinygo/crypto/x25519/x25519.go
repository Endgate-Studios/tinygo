// Package x25519 is the X25519 function of RFC 7748, constant time. The
// field arithmetic is tinygo/crypto/internal/field25519, so the RSA
// accelerator does the multiplications on the ESP32-S3. It does not allocate.
package x25519

import "tinygo/crypto/internal/field25519"

type fe = field25519.Element

var a24 = fe{121665}

// Basepoint is the u-coordinate 9 (RFC 7748 section 4.1).
var Basepoint = [32]byte{9}

// X25519 sets out to the scalar times the point with u-coordinate point
// (RFC 7748 section 5).
func X25519(out, scalar, point *[32]byte) {
	k := *scalar
	k[31] = k[31]&127 | 64
	k[0] &= 248
	var x1, x2, z2, x3, z3, a, aa, b, bb, e, c, d, da, cb fe
	x1.SetBytes(point)
	x2[0], x3, z3[0] = 1, x1, 1
	var swapped uint32
	for t := 254; t >= 0; t-- {
		bit := uint32(k[t>>3]>>(t&7)) & 1
		swapped ^= bit
		x2.Swap(&x3, swapped)
		z2.Swap(&z3, swapped)
		swapped = bit
		a.Add(&x2, &z2)
		aa.Mul(&a, &a)
		b.Sub(&x2, &z2)
		bb.Mul(&b, &b)
		e.Sub(&aa, &bb)
		c.Add(&x3, &z3)
		d.Sub(&x3, &z3)
		da.Mul(&d, &a)
		cb.Mul(&c, &b)
		x3.Add(&da, &cb)
		x3.Mul(&x3, &x3)
		z3.Sub(&da, &cb)
		z3.Mul(&z3, &z3)
		z3.Mul(&z3, &x1)
		x2.Mul(&aa, &bb)
		z2.Mul(&a24, &e)
		z2.Add(&z2, &aa)
		z2.Mul(&z2, &e)
	}
	x2.Swap(&x3, swapped)
	z2.Swap(&z3, swapped)
	z2.Invert(&z2)
	x2.Mul(&x2, &z2)
	x2.Bytes(out)
	clear(k[:])
	x1, x2, z2, x3, z3, a, aa, b, bb, e, c, d, da, cb = fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}
}
