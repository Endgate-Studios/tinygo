package ed25519

import (
	"encoding/binary"

	"tinygo/crypto/internal/mpi"
)

// scalar is a number below the group order L in 8 little-endian 32-bit
// words. The multiplication is tinygo/crypto/internal/mpi.
type scalar [8]uint32

var (
	// order is L = 2^252 + 27742317777372353535851937790883648493 (RFC 8032
	// section 5.1).
	order = scalar{0x5cf5d3ed, 0x5812631a, 0xa2f79cd6, 0x14def9de, 0, 0, 0, 0x10000000}
	modL  mpi.Modulus
	rrL   scalar
	c252  = scalar{7: 1 << 28} // 2^252, below L
	c504  scalar               // 2^504 mod L
)

func init() {
	modL.Init(order[:], rrL[:])
	modL.Mul(c504[:], c252[:], c252[:])
}

// add sets s = a + b mod L in constant time. s can be a or b.
func (s *scalar) add(a, b *scalar) {
	var sum, d scalar
	var c, borrow uint64
	for i := range sum {
		v := uint64(a[i]) + uint64(b[i]) + c
		sum[i], c = uint32(v), v>>32
	}
	for i := range d {
		v := uint64(sum[i]) - uint64(order[i]) - borrow
		d[i], borrow = uint32(v), (v>>32)&1
	}
	keep := -uint32(borrow) // all ones when sum < L; a + b < 2L < 2^256
	for i := range s {
		s[i] = sum[i]&keep | d[i]&^keep
	}
}

// reduce sets s = h mod L for the 512-bit little-endian h, in constant time:
// h = x0 + x1 2^252 + x2 2^504 with x0 and x1 below 2^252, so below L.
func (s *scalar) reduce(h *[64]byte) {
	var w [16]uint32
	for i := range w {
		w[i] = binary.LittleEndian.Uint32(h[4*i:])
	}
	var x0, x1, x2, t scalar
	copy(x0[:], w[:8])
	x0[7] &= 0x0fffffff
	for i := range x1 {
		x1[i] = w[7+i]>>28 | w[8+i]<<4
	}
	x1[7] &= 0x0fffffff
	x2[0] = w[15] >> 24
	modL.Mul(t[:], x1[:], c252[:])
	s.add(&x0, &t)
	modL.Mul(t[:], x2[:], c504[:])
	s.add(s, &t)
	w, x0, x1, x2, t = [16]uint32{}, scalar{}, scalar{}, scalar{}, scalar{}
}

// mulAdd sets s = a b + c mod L.
func (s *scalar) mulAdd(a, b, c *scalar) {
	var t scalar
	modL.Mul(t[:], a[:], b[:])
	s.add(&t, c)
	t = scalar{}
}

// bytes writes s to b, little endian.
func (s *scalar) bytes(b *[32]byte) {
	for i, v := range s {
		binary.LittleEndian.PutUint32(b[4*i:], v)
	}
}

// setCanonical sets s to the little-endian b and reports whether b is below
// L. b is public, so it need not be constant time.
func (s *scalar) setCanonical(b *[32]byte) bool {
	for i := range s {
		s[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] != order[i] {
			return s[i] < order[i]
		}
	}
	return false
}
