package p256

// ECDSA verification (FIPS 186-4 section 6.4.2) for TLS and certificates.
// All inputs are public, so the arithmetic modulo n is simple and not
// constant time.

// scalar is a value modulo n in 8 little-endian 32-bit limbs.
type scalar [8]uint32

var nScalar = scalarFromBytes(&n)

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

// mulModN sets r = a * b mod n by a shift and subtract of the 512-bit
// product.
func mulModN(r, a, b *scalar) {
	var p [16]uint32
	for i := 0; i < 8; i++ {
		var carry uint64
		for j := 0; j < 8; j++ {
			v := uint64(a[i])*uint64(b[j]) + uint64(p[i+j]) + carry
			p[i+j], carry = uint32(v), v>>32
		}
		p[i+8] = uint32(carry)
	}
	var acc scalar
	for i := 511; i >= 0; i-- {
		top := acc[7] >> 31
		for k := 7; k > 0; k-- {
			acc[k] = acc[k]<<1 | acc[k-1]>>31
		}
		acc[0] = acc[0]<<1 | p[i/32]>>(i%32)&1
		// acc < 2n, because acc < n before the shift.
		if top == 1 || geq(&acc, &nScalar) {
			sub(&acc, &nScalar)
		}
	}
	*r = acc
}

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
// bytes, as FIPS 186-4 section 6.4 says.
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
	var b1, b2 [32]byte
	u1.bytes(&b1)
	u2.bytes(&b2)
	var x1, y1, x2, y2, x, y [32]byte
	var err error
	if u1 == (scalar{}) {
		err = mul(mem, &b2, qx, qy, &x, &y)
	} else {
		err = mul(mem, &b1, nil, nil, &x1, &y1)
		if err == nil {
			err = mul(mem, &b2, qx, qy, &x2, &y2)
		}
		if err == nil {
			err = Add(&x1, &y1, &x2, &y2, &x, &y)
		}
	}
	if err != nil {
		return false
	}
	v := scalarFromBytes(&x)
	if geq(&v, &nScalar) {
		sub(&v, &nScalar)
	}
	return v == rs
}
