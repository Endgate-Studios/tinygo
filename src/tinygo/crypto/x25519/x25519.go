// Package x25519 is the X25519 function of RFC 7748, constant time, with the
// 16-limb field arithmetic of TweetNaCl (Bernstein, van Gastel, Janssen,
// Lange, Schwabe, Smetsers). It does not allocate.
package x25519

// fe is an element of GF(2^255 - 19) in 16 limbs of 16 bits.
type fe [16]int64

var a24 = fe{0xdb41, 1} // 121665

// Basepoint is the u-coordinate 9 (RFC 7748 section 4.1).
var Basepoint = [32]byte{9}

// carry reduces the limbs to 16 bits.
func carry(o *fe) {
	for i := 0; i < 16; i++ {
		o[i] += 1 << 16
		c := o[i] >> 16
		if i < 15 {
			o[i+1] += c - 1
		} else {
			o[0] += 38 * (c - 1)
		}
		o[i] -= c << 16
	}
}

// swap exchanges p and q when b is 1, in constant time.
func swap(p, q *fe, b int64) {
	c := ^(b - 1)
	for i := range p {
		t := c & (p[i] ^ q[i])
		p[i] ^= t
		q[i] ^= t
	}
}

func pack(o *[32]byte, n *fe) {
	t := *n
	var m fe
	carry(&t)
	carry(&t)
	carry(&t)
	for j := 0; j < 2; j++ {
		m[0] = t[0] - 0xffed
		for i := 1; i < 15; i++ {
			m[i] = t[i] - 0xffff - (m[i-1] >> 16 & 1)
			m[i-1] &= 0xffff
		}
		m[15] = t[15] - 0x7fff - (m[14] >> 16 & 1)
		b := m[15] >> 16 & 1
		m[14] &= 0xffff
		swap(&t, &m, 1-b)
	}
	for i := 0; i < 16; i++ {
		o[2*i] = byte(t[i])
		o[2*i+1] = byte(t[i] >> 8)
	}
}

func unpack(o *fe, n *[32]byte) {
	for i := 0; i < 16; i++ {
		o[i] = int64(n[2*i]) | int64(n[2*i+1])<<8
	}
	o[15] &= 0x7fff
}

func add(o, a, b *fe) {
	for i := range o {
		o[i] = a[i] + b[i]
	}
}

func sub(o, a, b *fe) {
	for i := range o {
		o[i] = a[i] - b[i]
	}
}

func mul(o, a, b *fe) {
	var t [31]int64
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			t[i+j] += a[i] * b[j]
		}
	}
	for i := 0; i < 15; i++ {
		t[i] += 38 * t[i+16]
	}
	copy(o[:], t[:16])
	carry(o)
	carry(o)
}

// inv sets o = i^(p-2).
func inv(o, i *fe) {
	c := *i
	for a := 253; a >= 0; a-- {
		mul(&c, &c, &c)
		if a != 2 && a != 4 {
			mul(&c, &c, i)
		}
	}
	*o = c
}

// X25519 sets out to the scalar times the point with u-coordinate point
// (RFC 7748 section 5).
func X25519(out, scalar, point *[32]byte) {
	z := *scalar
	z[31] = z[31]&127 | 64
	z[0] &= 248
	var x, a, b, c, d, e, f fe
	unpack(&x, point)
	b = x
	a[0], d[0] = 1, 1
	for i := 254; i >= 0; i-- {
		r := int64(z[i>>3] >> (i & 7) & 1)
		swap(&a, &b, r)
		swap(&c, &d, r)
		add(&e, &a, &c)
		sub(&a, &a, &c)
		add(&c, &b, &d)
		sub(&b, &b, &d)
		mul(&d, &e, &e)
		mul(&f, &a, &a)
		mul(&a, &c, &a)
		mul(&c, &b, &e)
		add(&e, &a, &c)
		sub(&a, &a, &c)
		mul(&b, &a, &a)
		sub(&c, &d, &f)
		mul(&a, &c, &a24)
		add(&a, &a, &d)
		mul(&c, &c, &a)
		mul(&a, &d, &f)
		mul(&d, &b, &x)
		mul(&b, &e, &e)
		swap(&a, &b, r)
		swap(&c, &d, r)
	}
	inv(&c, &c)
	mul(&a, &a, &c)
	pack(out, &a)
	clear(z[:])
	x, a, b, c, d, e, f = fe{}, fe{}, fe{}, fe{}, fe{}, fe{}, fe{}
}
