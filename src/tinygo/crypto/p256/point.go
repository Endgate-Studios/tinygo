package p256

// point is a projective point (X:Y:Z), with affine x = X/Z and y = Y/Z. The identity is (0:1:0).
type point struct {
	x, y, z element
}

// b is the curve coefficient (FIPS 186-4 appendix D.1.2.3), in limbs.
var b = element{0x27d2604b, 0x3bce3c3e, 0xcc53b0f6, 0x651d06b0, 0x769886bc, 0xb3ebbd55, 0xaa3a93e7, 0x5ac635d8}

// gen is the base point G.
var gen = point{
	x: element{0xd898c296, 0xf4a13945, 0x2deb33a0, 0x77037d81, 0x63a440f2, 0xf8bce6e5, 0xe12c4247, 0x6b17d1f2},
	y: element{0x37bf51f5, 0xcbb64068, 0x6b315ece, 0x2bce3357, 0x7c0f9e16, 0x8ee7eb4a, 0xfe1a7f9b, 0x4fe342e2},
	z: element{1},
}

func (q *point) setIdentity() {
	*q = point{y: element{1}}
}

// add sets q = p1 + p2 with the complete formula for a = -3 of Renes, Costello and Batina,
// "Complete addition formulas for prime order elliptic curves" (https://eprint.iacr.org/2015/1060), section A.2.
// It has no special cases, so it also doubles, and q may alias p1 or p2.
func (q *point) add(p1, p2 *point) {
	var t0, t1, t2, t3, t4, x3, y3, z3 element
	feMul(&t0, &p1.x, &p2.x)
	feMul(&t1, &p1.y, &p2.y)
	feMul(&t2, &p1.z, &p2.z)
	feAdd(&t3, &p1.x, &p1.y)
	feAdd(&t4, &p2.x, &p2.y)
	feMul(&t3, &t3, &t4)
	feAdd(&t4, &t0, &t1)
	feSub(&t3, &t3, &t4)
	feAdd(&t4, &p1.y, &p1.z)
	feAdd(&x3, &p2.y, &p2.z)
	feMul(&t4, &t4, &x3)
	feAdd(&x3, &t1, &t2)
	feSub(&t4, &t4, &x3)
	feAdd(&x3, &p1.x, &p1.z)
	feAdd(&y3, &p2.x, &p2.z)
	feMul(&x3, &x3, &y3)
	feAdd(&y3, &t0, &t2)
	feSub(&y3, &x3, &y3)
	feMul(&z3, &b, &t2)
	feSub(&x3, &y3, &z3)
	feAdd(&z3, &x3, &x3)
	feAdd(&x3, &x3, &z3)
	feSub(&z3, &t1, &x3)
	feAdd(&x3, &t1, &x3)
	feMul(&y3, &b, &y3)
	feAdd(&t1, &t2, &t2)
	feAdd(&t2, &t1, &t2)
	feSub(&y3, &y3, &t2)
	feSub(&y3, &y3, &t0)
	feAdd(&t1, &y3, &y3)
	feAdd(&y3, &t1, &y3)
	feAdd(&t1, &t0, &t0)
	feAdd(&t0, &t1, &t0)
	feSub(&t0, &t0, &t2)
	feMul(&t1, &t4, &y3)
	feMul(&t2, &t0, &y3)
	feMul(&y3, &x3, &z3)
	feAdd(&y3, &y3, &t2)
	feMul(&x3, &t3, &x3)
	feSub(&x3, &x3, &t1)
	feMul(&z3, &t4, &z3)
	feMul(&t1, &t3, &t0)
	feAdd(&z3, &z3, &t1)
	q.x, q.y, q.z = x3, y3, z3
	clear(t0[:])
	clear(t1[:])
	clear(t2[:])
	clear(t3[:])
	clear(t4[:])
	clear(x3[:])
	clear(y3[:])
	clear(z3[:])
}

// double sets q = 2*p with the exception-free doubling for a = -3 of Renes, Costello and Batina
// (https://eprint.iacr.org/2015/1060), algorithm 6. It costs less than add(p, p), and q may alias p.
func (q *point) double(p *point) {
	var t0, t1, t2, t3, x3, y3, z3 element
	feMul(&t0, &p.x, &p.x)
	feMul(&t1, &p.y, &p.y)
	feMul(&t2, &p.z, &p.z)
	feMul(&t3, &p.x, &p.y)
	feAdd(&t3, &t3, &t3)
	feMul(&z3, &p.x, &p.z)
	feAdd(&z3, &z3, &z3)
	feMul(&y3, &b, &t2)
	feSub(&y3, &y3, &z3)
	feAdd(&x3, &y3, &y3)
	feAdd(&y3, &x3, &y3)
	feSub(&x3, &t1, &y3)
	feAdd(&y3, &t1, &y3)
	feMul(&y3, &x3, &y3)
	feMul(&x3, &x3, &t3)
	feAdd(&t3, &t2, &t2)
	feAdd(&t2, &t2, &t3)
	feMul(&z3, &b, &z3)
	feSub(&z3, &z3, &t2)
	feSub(&z3, &z3, &t0)
	feAdd(&t3, &z3, &z3)
	feAdd(&z3, &z3, &t3)
	feAdd(&t3, &t0, &t0)
	feAdd(&t0, &t3, &t0)
	feSub(&t0, &t0, &t2)
	feMul(&t0, &t0, &z3)
	feAdd(&y3, &y3, &t0)
	feMul(&t0, &p.y, &p.z)
	feAdd(&t0, &t0, &t0)
	feMul(&z3, &t0, &z3)
	feSub(&x3, &x3, &z3)
	feMul(&z3, &t0, &t1)
	feAdd(&z3, &z3, &z3)
	feAdd(&z3, &z3, &z3)
	q.x, q.y, q.z = x3, y3, z3
	clear(t0[:])
	clear(t1[:])
	clear(t2[:])
	clear(t3[:])
	clear(x3[:])
	clear(y3[:])
	clear(z3[:])
}

// selectPoint sets q = a when bit is 1 and q = c when bit is 0, in constant time.
func (q *point) selectPoint(a, c *point, bit uint32) {
	feSelect(&q.x, &a.x, &c.x, bit)
	feSelect(&q.y, &a.y, &c.y, bit)
	feSelect(&q.z, &a.z, &c.z, bit)
}

// scalarMult sets acc = k * p with a fixed 4-bit window: table holds 0*p to 15*p, and each nibble
// of k costs four doublings, one scan of the whole table and one add. t is scratch. k is big endian.
func scalarMult(acc, t *point, table *[16]point, p *point, k *[32]byte) {
	table[0].setIdentity()
	table[1] = *p
	for i := 2; i < 16; i++ {
		table[i].add(&table[i-1], p)
	}
	acc.setIdentity()
	for i := 0; i < 64; i++ {
		acc.double(acc)
		acc.double(acc)
		acc.double(acc)
		acc.double(acc)
		nibble := uint32(k[i/2]>>(4-4*(i%2))) & 15
		// Read every entry, so the time does not depend on the nibble.
		t.setIdentity()
		for j := uint32(0); j < 16; j++ {
			d := j ^ nibble
			t.selectPoint(&table[j], t, ((d-1)>>31)&1) // 1 only when j == nibble
		}
		acc.add(acc, t)
	}
}

// affine writes x = X/Z and y = Y/Z, and reports false for the identity.
func (q *point) affine(x, y *element) bool {
	if feIsZero(&q.z) {
		return false
	}
	var zinv element
	feInv(&zinv, &q.z)
	feMul(x, &q.x, &zinv)
	feMul(y, &q.y, &zinv)
	clear(zinv[:])
	return true
}

// onCurve reports whether y^2 = x^3 - 3x + b.
func onCurve(x, y *element) bool {
	var lhs, rhs, t element
	feMul(&lhs, y, y)
	feMul(&rhs, x, x)
	feMul(&rhs, &rhs, x)
	feAdd(&t, x, x)
	feAdd(&t, &t, x)
	feSub(&rhs, &rhs, &t)
	feAdd(&rhs, &rhs, &b)
	return lhs == rhs
}
