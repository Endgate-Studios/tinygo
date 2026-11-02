package ed25519

import "tinygo/crypto/internal/field25519"

type fe = field25519.Element

// The curve constants of RFC 8032 section 5.1, from math/big (see
// TestConstants).
var (
	d2     = fe{0x26b2f159, 0xebd69b94, 0x8283b156, 0x00e0149a, 0xeef3d130, 0x198e80f2, 0x56dffce7, 0x2406d9dc} // 2d
	dConst = fe{0x135978a3, 0x75eb4dca, 0x4141d8ab, 0x00700a4d, 0x7779e898, 0x8cc74079, 0x2b6ffe73, 0x52036cee} // -121665/121666
	sqrtM1 = fe{0x4a0ea0b0, 0xc4ee1b27, 0xad2fe478, 0x2f431806, 0x3dfbd7a7, 0x2b4d0099, 0x4fc1df0b, 0x2b832480} // 2^((p-1)/4)
	base   = point{
		x: fe{0x8f25d51a, 0xc9562d60, 0x9525a7b2, 0x692cc760, 0xfdd6dc5c, 0xc0a4e231, 0xcd6e53fe, 0x216936d3},
		y: fe{0x66666658, 0x66666666, 0x66666666, 0x66666666, 0x66666666, 0x66666666, 0x66666666, 0x66666666},
		z: fe{1},
		t: fe{0xa5b7dda3, 0x6dde8ab3, 0x775152f5, 0x20f09f80, 0x64abe37d, 0x66ea4e8e, 0xd78b7665, 0x67875f0f},
	}
)

// point is a point of edwards25519 in extended coordinates (RFC 8032 section
// 5.1.4): x = X/Z, y = Y/Z and x y = T/Z.
type point struct{ x, y, z, t fe }

// cached is a point as the second operand of add: Y+X, Y-X, Z and 2d T.
type cached struct{ yPlusX, yMinusX, z, t2d fe }

// table holds 1P to 8P for signed digits of 4 bits.
type table [8]cached

func (p *point) identity() { *p = point{y: fe{1}, z: fe{1}} }

func (p *point) neg() {
	p.x.Neg(&p.x)
	p.t.Neg(&p.t)
}

func (c *cached) from(p *point) {
	c.yPlusX.Add(&p.y, &p.x)
	c.yMinusX.Sub(&p.y, &p.x)
	c.z = p.z
	c.t2d.Mul(&p.t, &d2)
}

// add sets r = p + q with the formulas of RFC 8032 section 5.1.4 for a = -1.
// r can be p.
func (r *point) add(p *point, q *cached) {
	var a, b, c, d, e, f, g, h fe
	a.Sub(&p.y, &p.x)
	a.Mul(&a, &q.yMinusX)
	b.Add(&p.y, &p.x)
	b.Mul(&b, &q.yPlusX)
	c.Mul(&p.t, &q.t2d)
	d.Mul(&p.z, &q.z)
	d.Add(&d, &d)
	e.Sub(&b, &a)
	f.Sub(&d, &c)
	g.Add(&d, &c)
	h.Add(&b, &a)
	r.x.Mul(&e, &f)
	r.y.Mul(&g, &h)
	r.t.Mul(&e, &h)
	r.z.Mul(&f, &g)
}

// double sets r = 2p with the doubling of RFC 8032 section 5.1.4. r can be p.
func (r *point) double(p *point) {
	var a, b, c, e, f, g, h fe
	a.Mul(&p.x, &p.x)
	b.Mul(&p.y, &p.y)
	c.Mul(&p.z, &p.z)
	c.Add(&c, &c)
	e.Add(&p.x, &p.y)
	e.Mul(&e, &e)
	h.Add(&a, &b)
	e.Sub(&e, &h)
	h.Neg(&h)
	g.Sub(&b, &a)
	f.Sub(&g, &c)
	r.x.Mul(&e, &f)
	r.y.Mul(&g, &h)
	r.t.Mul(&e, &h)
	r.z.Mul(&f, &g)
}

// bytes encodes p (RFC 8032 section 5.1.2).
func (p *point) bytes(out *[32]byte) {
	var zi, x, y fe
	zi.Invert(&p.z)
	x.Mul(&p.x, &zi)
	y.Mul(&p.y, &zi)
	y.Bytes(out)
	out[31] |= byte(x.IsNegative() << 7)
}

// setBytes decodes b (RFC 8032 section 5.1.3) and reports whether it is a
// point. As crypto/ed25519, it accepts y of p and more, and x = 0 with the
// sign bit set.
func (p *point) setBytes(b *[32]byte) bool {
	var y, yy, u, v, v3, x, vxx, t fe
	one := fe{1}
	y.SetBytes(b)
	yy.Mul(&y, &y)
	u.Sub(&yy, &one) // y^2 - 1
	v.Mul(&yy, &dConst)
	v.Add(&v, &one) // d y^2 + 1
	// x = u v^3 (u v^7)^((p-5)/8)
	v3.Mul(&v, &v)
	v3.Mul(&v3, &v)
	x.Mul(&v3, &v3)
	x.Mul(&x, &v)
	x.Mul(&x, &u)
	x.Pow22523(&x)
	x.Mul(&x, &v3)
	x.Mul(&x, &u)
	vxx.Mul(&x, &x)
	vxx.Mul(&vxx, &v)
	t.Neg(&u)
	if vxx.Equal(&u) == 0 {
		if vxx.Equal(&t) == 0 {
			return false
		}
		x.Mul(&x, &sqrtM1)
	}
	t.Neg(&x)
	x.Select(&t, x.IsNegative()^uint32(b[31]>>7))
	p.x, p.y, p.z = x, y, one
	p.t.Mul(&x, &y)
	return true
}

// init sets t to 1p to 8p.
func (t *table) init(p *point) {
	var q point
	t[0].from(p)
	q.double(p)
	t[1].from(&q)
	for i := 2; i < len(t); i++ {
		q.add(&q, &t[0])
		t[i].from(&q)
	}
	q = point{}
}

// lookup sets c to b times the point of t, for b in -8 to 8, in constant
// time.
func (t *table) lookup(c *cached, b int8) {
	neg := uint32(int32(b)) >> 31
	abs := uint32((int32(b) ^ -int32(neg)) + int32(neg))
	*c = cached{yPlusX: fe{1}, yMinusX: fe{1}, z: fe{1}}
	for j := range t {
		x := abs ^ uint32(j+1)
		eq := 1 ^ (x|-x)>>31
		c.yPlusX.Select(&t[j].yPlusX, eq)
		c.yMinusX.Select(&t[j].yMinusX, eq)
		c.z.Select(&t[j].z, eq)
		c.t2d.Select(&t[j].t2d, eq)
	}
	// -P has Y+X and Y-X swapped and -2d T.
	var nt fe
	nt.Neg(&c.t2d)
	c.t2d.Select(&nt, neg)
	c.yPlusX.Swap(&c.yMinusX, neg)
}

// digits writes k, which is below 2^255, as 64 signed digits in -8 to 7,
// least significant first, and the last up to 8 (ref10 ge_scalarmult_base).
func digits(e *[64]int8, k *[32]byte) {
	for i, v := range k {
		e[2*i], e[2*i+1] = int8(v&15), int8(v>>4)
	}
	var carry int8
	for i := 0; i < 63; i++ {
		e[i] += carry
		carry = (e[i] + 8) >> 4
		e[i] -= carry << 4
	}
	e[63] += carry
}

// scalarMultBase sets r = k B in constant time.
func scalarMultBase(r *point, k *scalar) {
	var t table
	var e [64]int8
	var kb [32]byte
	var c cached
	t.init(&base)
	k.bytes(&kb)
	digits(&e, &kb)
	r.identity()
	for i := 63; i >= 0; i-- {
		if i != 63 {
			r.double(r)
			r.double(r)
			r.double(r)
			r.double(r)
		}
		t.lookup(&c, e[i])
		r.add(r, &c)
	}
	t, e, kb, c = table{}, [64]int8{}, [32]byte{}, cached{}
}

// doubleScalarMult sets r = a A + b B. It is for public values.
func doubleScalarMult(r *point, a *scalar, A *point, b *scalar) {
	var ta, tb table
	var ea, eb [64]int8
	var kb [32]byte
	var c cached
	ta.init(A)
	tb.init(&base)
	a.bytes(&kb)
	digits(&ea, &kb)
	b.bytes(&kb)
	digits(&eb, &kb)
	r.identity()
	for i := 63; i >= 0; i-- {
		if i != 63 {
			r.double(r)
			r.double(r)
			r.double(r)
			r.double(r)
		}
		ta.lookup(&c, ea[i])
		r.add(r, &c)
		tb.lookup(&c, eb[i])
		r.add(r, &c)
	}
}
