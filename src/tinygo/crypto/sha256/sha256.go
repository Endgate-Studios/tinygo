// Package sha256 is the SHA-256 block function of FIPS 180-4 section 6.2.2,
// with a saved state, as HMAC needs. On the ESP32-S3 the SHA accelerator does
// the blocks. It does not call the GC heap.
package sha256

import "encoding/binary"

const (
	Size      = 32
	BlockSize = 64
)

// State is the chaining value H0 to H7 in the byte order of a digest.
type State [Size]byte

// Init is the initial hash value of FIPS 180-4 section 5.3.3.
var Init = State{
	0x6a, 0x09, 0xe6, 0x67, 0xbb, 0x67, 0xae, 0x85, 0x3c, 0x6e, 0xf3, 0x72, 0xa5, 0x4f, 0xf5, 0x3a,
	0x51, 0x0e, 0x52, 0x7f, 0x9b, 0x05, 0x68, 0x8c, 0x1f, 0x83, 0xd9, 0xab, 0x5b, 0xe0, 0xcd, 0x19,
}

// k is the constants of FIPS 180-4 section 4.2.2.
var k = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

// Digest hashes a message from a state. Use it as a value, it does not need
// a constructor.
type Digest struct {
	s   State
	buf [BlockSize]byte
	n   int
	len uint64
}

// Reset starts a message after done bytes that s already holds. done is a
// multiple of BlockSize, for example 0 with Init.
func (d *Digest) Reset(s State, done uint64) {
	d.s, d.n, d.len = s, 0, done
}

// Write adds p to the message.
func (d *Digest) Write(p []byte) {
	d.len += uint64(len(p))
	for len(p) > 0 {
		k := copy(d.buf[d.n:], p)
		d.n += k
		p = p[k:]
		if d.n == BlockSize {
			Block(&d.s, &d.buf)
			d.n = 0
		}
	}
}

// Sum writes the digest to out and clears the message buffer. Call Reset
// before the next message.
func (d *Digest) Sum(out *[Size]byte) {
	bits := d.len * 8
	d.buf[d.n] = 0x80
	d.n++
	if d.n > BlockSize-8 {
		clear(d.buf[d.n:])
		Block(&d.s, &d.buf)
		d.n = 0
	}
	clear(d.buf[d.n : BlockSize-8])
	binary.BigEndian.PutUint64(d.buf[BlockSize-8:], bits)
	Block(&d.s, &d.buf)
	*out = d.s
	clear(d.buf[:])
	clear(d.s[:])
}

// blockSoft is the SHA-256 compression of FIPS 180-4 section 6.2.2.
func blockSoft(s *State, p *[BlockSize]byte) {
	var w [64]uint32
	for i := 0; i < 16; i++ {
		w[i] = binary.BigEndian.Uint32(p[4*i:])
	}
	for i := 16; i < 64; i++ {
		v1, v2 := w[i-2], w[i-15]
		s1 := (v1>>17 | v1<<15) ^ (v1>>19 | v1<<13) ^ v1>>10
		s0 := (v2>>7 | v2<<25) ^ (v2>>18 | v2<<14) ^ v2>>3
		w[i] = s1 + w[i-7] + s0 + w[i-16]
	}
	var h [8]uint32
	for i := range h {
		h[i] = binary.BigEndian.Uint32(s[4*i:])
	}
	a, b, c, d, e, f, g, hh := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
	for i := 0; i < 64; i++ {
		t1 := hh + ((e>>6 | e<<26) ^ (e>>11 | e<<21) ^ (e>>25 | e<<7)) + (e&f ^ ^e&g) + k[i] + w[i]
		t2 := ((a>>2 | a<<30) ^ (a>>13 | a<<19) ^ (a>>22 | a<<10)) + (a&b ^ a&c ^ b&c)
		hh, g, f, e, d, c, b, a = g, f, e, d+t1, c, b, a, t1+t2
	}
	h[0] += a
	h[1] += b
	h[2] += c
	h[3] += d
	h[4] += e
	h[5] += f
	h[6] += g
	h[7] += hh
	for i := range h {
		binary.BigEndian.PutUint32(s[4*i:], h[i])
	}
	clear(w[:])
	clear(h[:])
}
