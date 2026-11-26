// Package field25519 is arithmetic in GF(2^255 - 19), constant time, for
// X25519 and Ed25519. The multiplication is tinygo/crypto/internal/mpi, so
// the RSA accelerator does it on the ESP32-S3. It does not allocate.
package field25519

import "tinygo/crypto/internal/mpi"

// Element is an element of GF(2^255 - 19) in 8 little-endian 32-bit words,
// always below p. The zero value is 0. The result of a method can be one of
// its arguments.
type Element [8]uint32

var (
	p    = Element{0xffffffed, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0x7fffffff}
	modP mpi.Modulus
	rr   Element

	// The exponents of Invert and Pow22523 for mpi.Exp: p - 2 and (p - 5)/8.
	pMinus2 = Element{0xffffffeb, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0x7fffffff}
	p58     = Element{0xfffffffd, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0x0fffffff}
)

func init() { modP.Init(p[:], rr[:]) }

// Mul sets z = a * b.
func (z *Element) Mul(a, b *Element) { modP.Mul(z[:], a[:], b[:]) }

// Square sets z = a^(2^n), for n at least 1.
func (z *Element) Square(a *Element, n int) {
	z.Mul(a, a)
	for i := 1; i < n; i++ {
		z.Mul(z, z)
	}
}

// Neg sets z = -a.
func (z *Element) Neg(a *Element) {
	var zero Element
	z.Sub(&zero, a)
}

// reduce subtracts p from z when z is at least p, with a mask. z must be
// below 2p.
func (z *Element) reduce() {
	var d Element
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

// SetBytes sets z to the little-endian b with the top bit ignored, reduced
// modulo p (RFC 7748 section 5, RFC 8032 section 5.1.3 as most
// implementations decode).
func (z *Element) SetBytes(b *[32]byte) {
	for i := range z {
		z[i] = uint32(b[4*i]) | uint32(b[4*i+1])<<8 | uint32(b[4*i+2])<<16 | uint32(b[4*i+3])<<24
	}
	z[7] &= 0x7fffffff
	z.reduce()
}

// Bytes writes z to b, little endian.
func (z *Element) Bytes(b *[32]byte) {
	for i, w := range z {
		b[4*i], b[4*i+1], b[4*i+2], b[4*i+3] = byte(w), byte(w>>8), byte(w>>16), byte(w>>24)
	}
}

// Swap exchanges z and b when bit is 1, in constant time. bit is 0 or 1.
func (z *Element) Swap(b *Element, bit uint32) {
	mask := -bit
	for i := range z {
		t := mask & (z[i] ^ b[i])
		z[i] ^= t
		b[i] ^= t
	}
}

// Select sets z = a when bit is 1, in constant time. bit is 0 or 1.
func (z *Element) Select(a *Element, bit uint32) {
	mask := -bit
	for i := range z {
		z[i] ^= mask & (z[i] ^ a[i])
	}
}

// Equal gives 1 when z = a and 0 otherwise, in constant time.
func (z *Element) Equal(a *Element) uint32 {
	var d uint32
	for i := range z {
		d |= z[i] ^ a[i]
	}
	return 1 ^ (d|-d)>>31
}

// IsNegative gives the least significant bit of z, the sign of RFC 8032
// section 5.1.2.
func (z *Element) IsNegative() uint32 { return z[0] & 1 }

// chain sets t = a^(2^250 - 1) and z11 = a^11 with the addition chain of
// ref10 fe_invert and fe_pow22523.
func chain(t, z11, a *Element) {
	var z2, z9, z5, z10, z20, z50, z100 Element
	z2.Mul(a, a)
	t.Square(&z2, 2)
	z9.Mul(t, a)
	z11.Mul(&z9, &z2)
	t.Mul(z11, z11)
	z5.Mul(t, &z9) // 2^5 - 1
	t.Square(&z5, 5)
	z10.Mul(t, &z5) // 2^10 - 1
	t.Square(&z10, 10)
	z20.Mul(t, &z10) // 2^20 - 1
	t.Square(&z20, 20)
	t.Mul(t, &z20) // 2^40 - 1
	t.Square(t, 10)
	z50.Mul(t, &z10) // 2^50 - 1
	t.Square(&z50, 50)
	z100.Mul(t, &z50) // 2^100 - 1
	t.Square(&z100, 100)
	t.Mul(t, &z100) // 2^200 - 1
	t.Square(t, 50)
	t.Mul(t, &z50) // 2^250 - 1
	z2, z9, z5, z10, z20, z50, z100 = Element{}, Element{}, Element{}, Element{}, Element{}, Element{}, Element{}
}

// Invert sets z = a^(p-2), which is 1/a, or 0 for 0. Without the hardware,
// the addition chain needs fewer multiplications than Exp.
func (z *Element) Invert(a *Element) {
	if mpi.HardwareExp {
		modP.Exp(z[:], a[:], pMinus2[:])
		return
	}
	var t, z11 Element
	chain(&t, &z11, a)
	t.Square(&t, 5)
	z.Mul(&t, &z11) // 2^255 - 21
	t, z11 = Element{}, Element{}
}

// Pow22523 sets z = a^((p-5)/8), the power of a square root (RFC 8032
// section 5.1.3).
func (z *Element) Pow22523(a *Element) {
	if mpi.HardwareExp {
		modP.Exp(z[:], a[:], p58[:])
		return
	}
	var t, z11 Element
	chain(&t, &z11, a)
	t.Square(&t, 2)
	z.Mul(&t, a) // 2^252 - 3
	t, z11 = Element{}, Element{}
}
