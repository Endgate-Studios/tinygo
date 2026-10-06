// Package sha512 is the SHA-512 block function of FIPS 180-4 section 6.4.2,
// with a saved state, and SHA-384 and SHA-512 on top of it. On the ESP32-S3
// the SHA accelerator does the blocks. It does not call the GC heap.
package sha512

import "encoding/binary"

const (
	Size      = 64
	Size384   = 48
	BlockSize = 128
)

// State is the chaining value H0 to H7 in the byte order of a digest.
type State [Size]byte

// The initial hash values of FIPS 180-4 sections 5.3.4 and 5.3.5.
var (
	Init384 = stateOf([8]uint64{
		0xcbbb9d5dc1059ed8, 0x629a292a367cd507, 0x9159015a3070dd17, 0x152fecd8f70e5939,
		0x67332667ffc00b31, 0x8eb44a8768581511, 0xdb0c2e0d64f98fa7, 0x47b5481dbefa4fa4,
	})
	Init512 = stateOf([8]uint64{
		0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
		0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
	})
)

// k is the constants of FIPS 180-4 section 4.2.3.
var k = [80]uint64{
	0x428a2f98d728ae22, 0x7137449123ef65cd, 0xb5c0fbcfec4d3b2f, 0xe9b5dba58189dbbc,
	0x3956c25bf348b538, 0x59f111f1b605d019, 0x923f82a4af194f9b, 0xab1c5ed5da6d8118,
	0xd807aa98a3030242, 0x12835b0145706fbe, 0x243185be4ee4b28c, 0x550c7dc3d5ffb4e2,
	0x72be5d74f27b896f, 0x80deb1fe3b1696b1, 0x9bdc06a725c71235, 0xc19bf174cf692694,
	0xe49b69c19ef14ad2, 0xefbe4786384f25e3, 0x0fc19dc68b8cd5b5, 0x240ca1cc77ac9c65,
	0x2de92c6f592b0275, 0x4a7484aa6ea6e483, 0x5cb0a9dcbd41fbd4, 0x76f988da831153b5,
	0x983e5152ee66dfab, 0xa831c66d2db43210, 0xb00327c898fb213f, 0xbf597fc7beef0ee4,
	0xc6e00bf33da88fc2, 0xd5a79147930aa725, 0x06ca6351e003826f, 0x142929670a0e6e70,
	0x27b70a8546d22ffc, 0x2e1b21385c26c926, 0x4d2c6dfc5ac42aed, 0x53380d139d95b3df,
	0x650a73548baf63de, 0x766a0abb3c77b2a8, 0x81c2c92e47edaee6, 0x92722c851482353b,
	0xa2bfe8a14cf10364, 0xa81a664bbc423001, 0xc24b8b70d0f89791, 0xc76c51a30654be30,
	0xd192e819d6ef5218, 0xd69906245565a910, 0xf40e35855771202a, 0x106aa07032bbd1b8,
	0x19a4c116b8d2d0c8, 0x1e376c085141ab53, 0x2748774cdf8eeb99, 0x34b0bcb5e19b48a8,
	0x391c0cb3c5c95a63, 0x4ed8aa4ae3418acb, 0x5b9cca4f7763e373, 0x682e6ff3d6b2b8a3,
	0x748f82ee5defb2fc, 0x78a5636f43172f60, 0x84c87814a1f0ab72, 0x8cc702081a6439ec,
	0x90befffa23631e28, 0xa4506cebde82bde9, 0xbef9a3f7b2c67915, 0xc67178f2e372532b,
	0xca273eceea26619c, 0xd186b8c721c0c207, 0xeada7dd6cde0eb1e, 0xf57d4f7fee6ed178,
	0x06f067aa72176fba, 0x0a637dc5a2c898a6, 0x113f9804bef90dae, 0x1b710b35131c471b,
	0x28db77f523047d84, 0x32caab7b40c72493, 0x3c9ebe0a15c9bebc, 0x431d67c49c100d4c,
	0x4cc5d4becb3e42b6, 0x597f299cfc657e2a, 0x5fcb6fab3ad6faec, 0x6c44198c4a475817,
}

func stateOf(h [8]uint64) State {
	var s State
	for i, v := range h {
		binary.BigEndian.PutUint64(s[8*i:], v)
	}
	return s
}

// Digest hashes a message from a state. Use it as a value.
type Digest struct {
	s   State
	buf [BlockSize]byte
	n   int
	len uint64
}

// Reset starts a message after done bytes that s already holds. done is a
// multiple of BlockSize, for example 0 with Init384.
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

// Sum writes the final state to out and clears the message buffer. For
// SHA-384, the digest is the first Size384 bytes.
func (d *Digest) Sum(out *[Size]byte) {
	bits := d.len * 8
	d.buf[d.n] = 0x80
	d.n++
	if d.n > BlockSize-16 {
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

// Sum384 writes the SHA-384 digest of msg to out.
func Sum384(out *[Size384]byte, msg []byte) {
	var d Digest
	d.Reset(Init384, 0)
	d.Write(msg)
	var full [Size]byte
	d.Sum(&full)
	copy(out[:], full[:Size384])
	clear(full[:])
}

// blockSoft is the SHA-512 compression of FIPS 180-4 section 6.4.2.
func blockSoft(s *State, p *[BlockSize]byte) {
	var w [80]uint64
	for i := 0; i < 16; i++ {
		w[i] = binary.BigEndian.Uint64(p[8*i:])
	}
	for i := 16; i < 80; i++ {
		v1, v2 := w[i-2], w[i-15]
		s1 := (v1>>19 | v1<<45) ^ (v1>>61 | v1<<3) ^ v1>>6
		s0 := (v2>>1 | v2<<63) ^ (v2>>8 | v2<<56) ^ v2>>7
		w[i] = s1 + w[i-7] + s0 + w[i-16]
	}
	var h [8]uint64
	for i := range h {
		h[i] = binary.BigEndian.Uint64(s[8*i:])
	}
	a, b, c, d, e, f, g, hh := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
	for i := 0; i < 80; i++ {
		t1 := hh + ((e>>14 | e<<50) ^ (e>>18 | e<<46) ^ (e>>41 | e<<23)) + (e&f ^ ^e&g) + k[i] + w[i]
		t2 := ((a>>28 | a<<36) ^ (a>>34 | a<<30) ^ (a>>39 | a<<25)) + (a&b ^ a&c ^ b&c)
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
		binary.BigEndian.PutUint64(s[8*i:], h[i])
	}
	clear(w[:])
	clear(h[:])
}
