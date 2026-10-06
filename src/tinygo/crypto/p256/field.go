package p256

// element is a field element modulo p in 8 little-endian 32-bit limbs, always below p.
type element [8]uint32

// p = 2^256 - 2^224 + 2^192 + 2^96 - 1 (FIPS 186-4 appendix D.1.2.3).
var pElem = element{0xffffffff, 0xffffffff, 0xffffffff, 0, 0, 0, 1, 0xffffffff}

// feAdd sets r = a + b mod p in constant time.
func feAdd(r, a, b *element) {
	var s element
	var carry uint64
	for i := range s {
		v := uint64(a[i]) + uint64(b[i]) + carry
		s[i], carry = uint32(v), v>>32
	}
	subtractP(r, &s, uint32(carry))
}

// subtractP sets r = s - p when s plus carry*2^256 is at least p, and r = s otherwise.
func subtractP(r, s *element, carry uint32) {
	var d element
	var borrow uint64
	for i := range d {
		v := uint64(s[i]) - uint64(pElem[i]) - borrow
		d[i], borrow = uint32(v), (v>>32)&1
	}
	mask := -(carry | uint32(borrow^1))
	for i := range r {
		r[i] = d[i]&mask | s[i]&^mask
	}
}

// feSub sets r = a - b mod p in constant time.
func feSub(r, a, b *element) {
	var d element
	var borrow uint64
	for i := range d {
		v := uint64(a[i]) - uint64(b[i]) - borrow
		d[i], borrow = uint32(v), (v>>32)&1
	}
	mask := -uint32(borrow)
	var carry uint64
	for i := range r {
		v := uint64(d[i]) + uint64(pElem[i]&mask) + carry
		r[i], carry = uint32(v), v>>32
	}
}

// feInv sets r = a^(p-2) mod p, which is the inverse of a when a is not 0. The exponent is public,
// so the square and multiply pattern does not depend on secrets.
func feInv(r, a *element) {
	e := element{0xfffffffd, 0xffffffff, 0xffffffff, 0, 0, 0, 1, 0xffffffff} // p - 2
	x := element{1}
	for i := 255; i >= 0; i-- {
		feMul(&x, &x, &x)
		if e[i/32]>>(i%32)&1 == 1 {
			feMul(&x, &x, a)
		}
	}
	*r = x
	clear(x[:])
}

// feSelect sets r = a when bit is 1 and r = b when bit is 0, in constant time.
func feSelect(r, a, b *element, bit uint32) {
	mask := -bit
	for i := range r {
		r[i] = a[i]&mask | b[i]&^mask
	}
}

func feIsZero(a *element) bool {
	var acc uint32
	for _, v := range a {
		acc |= v
	}
	return acc == 0
}

// feFromBytes reads a big-endian 32-byte value, and reports false when it is not below p.
func feFromBytes(r *element, b *[32]byte) bool {
	for i := 0; i < 8; i++ {
		o := 28 - 4*i
		r[i] = uint32(b[o])<<24 | uint32(b[o+1])<<16 | uint32(b[o+2])<<8 | uint32(b[o+3])
	}
	var d element
	var borrow uint64
	for i := range d {
		v := uint64(r[i]) - uint64(pElem[i]) - borrow
		d[i], borrow = uint32(v), (v>>32)&1
	}
	return borrow == 1
}

func feToBytes(b *[32]byte, a *element) {
	for i := 0; i < 8; i++ {
		o := 28 - 4*i
		b[o], b[o+1], b[o+2], b[o+3] = byte(a[i]>>24), byte(a[i]>>16), byte(a[i]>>8), byte(a[i])
	}
}
