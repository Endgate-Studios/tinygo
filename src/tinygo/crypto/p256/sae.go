package p256

// The operations that SAE needs (IEEE 802.11-2020 12.4.4.2) on top of ECDH. All values are big
// endian. The exponents are public, so their square and multiply pattern does not depend on secrets.

// ErrIdentity tells that a sum is the point at infinity.
const ErrIdentity Error = "p256: the sum is the point at infinity"

var (
	sqrtExp     = element{0x00000000, 0x00000000, 0x40000000, 0x00000000, 0x00000000, 0x40000000, 0xc0000000, 0x3fffffff} // (p+1)/4
	legendreExp = element{0xffffffff, 0xffffffff, 0x7fffffff, 0x00000000, 0x00000000, 0x80000000, 0x80000000, 0x7fffffff} // (p-1)/2
)

// BelowP reports whether v is below the prime p.
func BelowP(v *[32]byte) bool {
	var e element
	ok := feFromBytes(&e, v)
	clear(e[:])
	return ok
}

// YSquared sets out = x^3 - 3x + b, the right side of the curve equation. It reports false when
// x is not below p.
func YSquared(x, out *[32]byte) bool {
	var ex, r, t element
	if !feFromBytes(&ex, x) {
		return false
	}
	feMul(&r, &ex, &ex)
	feMul(&r, &r, &ex)
	feAdd(&t, &ex, &ex)
	feAdd(&t, &t, &ex)
	feSub(&r, &r, &t)
	feAdd(&r, &r, &b)
	feToBytes(out, &r)
	clear(ex[:])
	clear(r[:])
	clear(t[:])
	return true
}

// IsSquare reports whether a, below p, is a quadratic residue modulo p (Euler's criterion). The
// time does not depend on a.
func IsSquare(a *[32]byte) bool {
	var ea, r element
	feFromBytes(&ea, a)
	fePow(&r, &ea, &legendreExp)
	one := element{1}
	ok := r == one || feIsZero(&ea)
	clear(ea[:])
	clear(r[:])
	return ok
}

// Sqrt sets out to a square root of a modulo p, which is a^((p+1)/4) because p = 3 mod 4. It
// reports false when a has no root.
func Sqrt(a, out *[32]byte) bool {
	var ea, r, c element
	if !feFromBytes(&ea, a) {
		return false
	}
	fePow(&r, &ea, &sqrtExp)
	feMul(&c, &r, &r)
	ok := c == ea
	feToBytes(out, &r)
	clear(ea[:])
	clear(r[:])
	clear(c[:])
	return ok
}

// NegateY sets out = p - y, the y coordinate of the inverse point. y must be below p.
func NegateY(y, out *[32]byte) {
	var ey, r element
	feFromBytes(&ey, y)
	feSub(&r, &element{}, &ey)
	feToBytes(out, &r)
	clear(ey[:])
	clear(r[:])
}

// ScalarMult computes k times the point (x, y). k must be in 1 to n-1.
func ScalarMult(mem Memory, k, x, y, outX, outY *[32]byte) error {
	return mul(mem, k, x, y, outX, outY)
}

// Add computes the sum of two points on the curve.
func Add(x1, y1, x2, y2, outX, outY *[32]byte) error {
	var p1, p2, s point
	var x, y element
	var err error = ErrPoint
	if feFromBytes(&p1.x, x1) && feFromBytes(&p1.y, y1) && onCurve(&p1.x, &p1.y) &&
		feFromBytes(&p2.x, x2) && feFromBytes(&p2.y, y2) && onCurve(&p2.x, &p2.y) {
		p1.z, p2.z = element{1}, element{1}
		s.add(&p1, &p2)
		err = ErrIdentity
		if s.affine(&x, &y) {
			feToBytes(outX, &x)
			feToBytes(outY, &y)
			err = nil
		}
	}
	p1, p2, s = point{}, point{}, point{}
	clear(x[:])
	clear(y[:])
	return err
}

// ValidScalar reports in constant time whether 0 < k < n.
func ValidScalar(k *[32]byte) bool { return validScalar(k) }

// AddModN sets out = (a + b) mod n for a and b below n, in constant time.
func AddModN(a, b, out *[32]byte) {
	var s, d [32]byte
	var carry int
	for i := 31; i >= 0; i-- {
		v := int(a[i]) + int(b[i]) + carry
		s[i], carry = byte(v), v>>8
	}
	borrow := 0
	for i := 31; i >= 0; i-- {
		v := int(s[i]) - int(n[i]) - borrow
		d[i], borrow = byte(v), (v>>8)&1
	}
	// Take s - n when the sum carried out or is at least n.
	mask := -byte(carry | (borrow ^ 1))
	for i := range out {
		out[i] = d[i]&mask | s[i]&^mask
	}
	clear(s[:])
	clear(d[:])
}
