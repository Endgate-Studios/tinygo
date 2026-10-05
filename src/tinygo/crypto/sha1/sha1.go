// Package sha1 is the SHA-1 block function of FIPS 180-4 section 6.1.2, with
// a saved state, as HMAC needs. On the ESP32-S3 the SHA accelerator does the
// blocks. It does not call the GC heap.
package sha1

import "encoding/binary"

const (
	Size      = 20
	BlockSize = 64
)

// State is the chaining value H0 to H4 in the byte order of a digest.
type State [Size]byte

// Init is the initial hash value of FIPS 180-4 section 5.3.1.
var Init = State{
	0x67, 0x45, 0x23, 0x01, 0xef, 0xcd, 0xab, 0x89, 0x98, 0xba,
	0xdc, 0xfe, 0x10, 0x32, 0x54, 0x76, 0xc3, 0xd2, 0xe1, 0xf0,
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

// blockSoft is the SHA-1 compression of FIPS 180-4 section 6.1.2.
func blockSoft(s *State, p *[BlockSize]byte) {
	var w [16]uint32
	for i := range w {
		w[i] = binary.BigEndian.Uint32(p[4*i:])
	}
	h0 := binary.BigEndian.Uint32(s[0:])
	h1 := binary.BigEndian.Uint32(s[4:])
	h2 := binary.BigEndian.Uint32(s[8:])
	h3 := binary.BigEndian.Uint32(s[12:])
	h4 := binary.BigEndian.Uint32(s[16:])
	a, b, c, d, e := h0, h1, h2, h3, h4
	for t := 0; t < 80; t++ {
		if t >= 16 {
			x := w[(t-3)&15] ^ w[(t-8)&15] ^ w[(t-14)&15] ^ w[t&15]
			w[t&15] = x<<1 | x>>31
		}
		var f, k uint32
		switch {
		case t < 20:
			f, k = b&c|^b&d, 0x5a827999
		case t < 40:
			f, k = b^c^d, 0x6ed9eba1
		case t < 60:
			f, k = b&c|b&d|c&d, 0x8f1bbcdc
		default:
			f, k = b^c^d, 0xca62c1d6
		}
		tmp := a<<5 | a>>27 + f + e + k + w[t&15]
		a, b, c, d, e = tmp, a, b<<30|b>>2, c, d
	}
	binary.BigEndian.PutUint32(s[0:], h0+a)
	binary.BigEndian.PutUint32(s[4:], h1+b)
	binary.BigEndian.PutUint32(s[8:], h2+c)
	binary.BigEndian.PutUint32(s[12:], h3+d)
	binary.BigEndian.PutUint32(s[16:], h4+e)
	clear(w[:])
}
