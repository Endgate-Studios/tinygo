package chacha20poly1305

import "encoding/binary"

// poly1305 is the Poly1305 MAC of RFC 8439 section 2.5, with the
// accumulator in five 26-bit limbs and 32x32-bit products, as poly1305-donna
// for 32-bit processors. Its time does not depend on the data.
type poly1305 struct {
	r0, r1, r2, r3, r4 uint32
	s1, s2, s3, s4     uint32 // 5 r
	h0, h1, h2, h3, h4 uint32
	pad                [4]uint32
}

const mask26 = 0x3ffffff

// init sets the key: r, clamped, from k[0:4] and s from k[4:8] (section 2.5).
func (p *poly1305) init(k *[16]uint32) {
	*p = poly1305{
		r0:  k[0] & mask26,
		r1:  (k[0]>>26 | k[1]<<6) & 0x3ffff03,
		r2:  (k[1]>>20 | k[2]<<12) & 0x3ffc0ff,
		r3:  (k[2]>>14 | k[3]<<18) & 0x3f03fff,
		r4:  k[3] >> 8 & 0x00fffff,
		pad: [4]uint32{k[4], k[5], k[6], k[7]},
	}
	p.s1, p.s2, p.s3, p.s4 = p.r1*5, p.r2*5, p.r3*5, p.r4*5
}

// padded adds b to the message, with zeros up to a multiple of 16 bytes
// (pad16 of section 2.8).
func (p *poly1305) padded(b []byte) {
	for len(b) >= 16 {
		p.block(b)
		b = b[16:]
	}
	if len(b) > 0 {
		var last [16]byte
		copy(last[:], b)
		p.block(last[:])
	}
}

// block adds the 16 bytes of m with the bit 2^128.
func (p *poly1305) block(m []byte) {
	m0, m1 := binary.LittleEndian.Uint32(m), binary.LittleEndian.Uint32(m[4:])
	m2, m3 := binary.LittleEndian.Uint32(m[8:]), binary.LittleEndian.Uint32(m[12:])
	h0 := p.h0 + m0&mask26
	h1 := p.h1 + (m0>>26|m1<<6)&mask26
	h2 := p.h2 + (m1>>20|m2<<12)&mask26
	h3 := p.h3 + (m2>>14|m3<<18)&mask26
	h4 := p.h4 + (m3>>8 | 1<<24)
	r0, r1, r2, r3, r4 := uint64(p.r0), uint64(p.r1), uint64(p.r2), uint64(p.r3), uint64(p.r4)
	s1, s2, s3, s4 := uint64(p.s1), uint64(p.s2), uint64(p.s3), uint64(p.s4)
	d0 := uint64(h0)*r0 + uint64(h1)*s4 + uint64(h2)*s3 + uint64(h3)*s2 + uint64(h4)*s1
	d1 := uint64(h0)*r1 + uint64(h1)*r0 + uint64(h2)*s4 + uint64(h3)*s3 + uint64(h4)*s2
	d2 := uint64(h0)*r2 + uint64(h1)*r1 + uint64(h2)*r0 + uint64(h3)*s4 + uint64(h4)*s3
	d3 := uint64(h0)*r3 + uint64(h1)*r2 + uint64(h2)*r1 + uint64(h3)*r0 + uint64(h4)*s4
	d4 := uint64(h0)*r4 + uint64(h1)*r3 + uint64(h2)*r2 + uint64(h3)*r1 + uint64(h4)*r0
	d1 += d0 >> 26
	d2 += d1 >> 26
	d3 += d2 >> 26
	d4 += d3 >> 26
	c := uint32(d4 >> 26)
	h0 = uint32(d0)&mask26 + c*5
	p.h0 = h0 & mask26
	p.h1 = uint32(d1)&mask26 + h0>>26
	p.h2 = uint32(d2) & mask26
	p.h3 = uint32(d3) & mask26
	p.h4 = uint32(d4) & mask26
}

// tag computes the tag of section 2.8 over aad and ct.
func (p *poly1305) tag(out *[Overhead]byte, aad, ct []byte) {
	p.padded(aad)
	p.padded(ct)
	var lens [16]byte
	binary.LittleEndian.PutUint64(lens[:], uint64(len(aad)))
	binary.LittleEndian.PutUint64(lens[8:], uint64(len(ct)))
	p.block(lens[:])

	// Carry fully, then h - p when h >= p, with a mask.
	h0, h1, h2, h3, h4 := p.h0, p.h1, p.h2, p.h3, p.h4
	h2 += h1 >> 26
	h1 &= mask26
	h3 += h2 >> 26
	h2 &= mask26
	h4 += h3 >> 26
	h3 &= mask26
	h0 += (h4 >> 26) * 5
	h4 &= mask26
	h1 += h0 >> 26
	h0 &= mask26
	g0 := h0 + 5
	g1 := h1 + g0>>26
	g0 &= mask26
	g2 := h2 + g1>>26
	g1 &= mask26
	g3 := h3 + g2>>26
	g2 &= mask26
	g4 := h4 + g3>>26 - 1<<26
	g3 &= mask26
	keep := (g4 >> 31) - 1 // all ones when h + 5 reached 2^130
	h0 = h0&^keep | g0&keep
	h1 = h1&^keep | g1&keep
	h2 = h2&^keep | g2&keep
	h3 = h3&^keep | g3&keep
	h4 = h4&^keep | g4&keep

	// h mod 2^128 plus s.
	f := uint64(h0|h1<<26) + uint64(p.pad[0])
	binary.LittleEndian.PutUint32(out[0:], uint32(f))
	f = uint64(h1>>6|h2<<20) + uint64(p.pad[1]) + f>>32
	binary.LittleEndian.PutUint32(out[4:], uint32(f))
	f = uint64(h2>>12|h3<<14) + uint64(p.pad[2]) + f>>32
	binary.LittleEndian.PutUint32(out[8:], uint32(f))
	f = uint64(h3>>18|h4<<8) + uint64(p.pad[3]) + f>>32
	binary.LittleEndian.PutUint32(out[12:], uint32(f))
}
