package aes

import (
	"crypto/subtle"
)

// GCM of NIST SP 800-38D with a 12-byte nonce and a 16-byte tag, as TLS 1.3
// uses it. It does not allocate. GHASH is in software, and its time does not
// depend on the data.

const (
	NonceSize = 12
	TagSize   = 16
)

// ErrAuth tells that the tag of a message does not match.
const ErrAuth Error = "aes: message authentication failed"

// GCM is AES-GCM with the key of a Cipher. Use it as a value.
type GCM struct {
	c *Cipher
	h [4]uint32 // the hash key E(K, 0), with the last word first
}

// Init sets the cipher. The GCM uses c until c is closed.
func (g *GCM) Init(c *Cipher) error {
	if c.rk == nil {
		return ErrClosed
	}
	g.c = c
	var h [BlockSize]byte
	c.encrypt(&h, &h)
	g.h = load(&h)
	clear(h[:])
	return nil
}

// Clear zeroes the hash key.
func (g *GCM) Clear() { clear(g.h[:]) }

// Seal encrypts plain into dst and appends the tag over aad and the
// ciphertext. len(dst) must be at least len(plain) + TagSize. dst and plain
// can start at the same byte.
func (g *GCM) Seal(dst []byte, nonce *[NonceSize]byte, plain, aad []byte) {
	g.ctr(dst[:len(plain)], plain, nonce)
	var tag [TagSize]byte
	g.tag(&tag, nonce, aad, dst[:len(plain)])
	copy(dst[len(plain):], tag[:])
}

// Open checks the tag at the end of sealed and decrypts the rest into dst. It
// gives ErrAuth, and writes nothing, when the tag does not match. dst and
// sealed can start at the same byte.
func (g *GCM) Open(dst []byte, nonce *[NonceSize]byte, sealed, aad []byte) error {
	if len(sealed) < TagSize {
		return ErrAuth
	}
	ct := sealed[:len(sealed)-TagSize]
	var tag [TagSize]byte
	g.tag(&tag, nonce, aad, ct)
	ok := subtle.ConstantTimeCompare(tag[:], sealed[len(ct):]) == 1
	clear(tag[:])
	if !ok {
		return ErrAuth
	}
	g.ctr(dst[:len(ct)], ct, nonce)
	return nil
}

// ctr is GCTR from the counter block J0 + 1 (section 6.5).
func (g *GCM) ctr(dst, src []byte, nonce *[NonceSize]byte) {
	var cb [BlockSize]byte
	copy(cb[:], nonce[:])
	cb[15] = 2
	g.c.ctr(dst, src, &cb)
}

// ctrSoft is ctr with one block at a time.
func (c *Cipher) ctrSoft(dst, src []byte, cb *[BlockSize]byte) {
	var ks [BlockSize]byte
	for len(src) > 0 {
		c.encrypt(&ks, cb)
		n := xorBlock(dst, src, &ks)
		dst, src = dst[n:], src[n:]
		incCounter(cb)
	}
	clear(ks[:])
}

// xorBlock sets dst to src XOR ks for up to one block, and gives the length.
func xorBlock(dst, src []byte, ks *[BlockSize]byte) int {
	n := min(len(src), BlockSize)
	dst, src = dst[:n], src[:n]
	for i := range src {
		dst[i] = src[i] ^ ks[i]
	}
	return n
}

// incCounter increments the last 4 bytes of cb, big endian.
func incCounter(cb *[BlockSize]byte) {
	for i := 15; i >= 12; i-- {
		cb[i]++
		if cb[i] != 0 {
			return
		}
	}
}

// tag computes GHASH over aad and ct, and encrypts it with the block J0
// (section 7.1).
func (g *GCM) tag(out *[TagSize]byte, nonce *[NonceSize]byte, aad, ct []byte) {
	var y [4]uint32
	g.ghash(&y, aad)
	g.ghash(&y, ct)
	aadBits, ctBits := uint64(len(aad))*8, uint64(len(ct))*8
	y[3] ^= uint32(aadBits >> 32)
	y[2] ^= uint32(aadBits)
	y[1] ^= uint32(ctBits >> 32)
	y[0] ^= uint32(ctBits)
	g.mul(&y)
	var j0 [BlockSize]byte
	copy(j0[:], nonce[:])
	j0[15] = 1
	g.c.encrypt(&j0, &j0)
	for i := range 4 {
		w := y[3-i]
		out[4*i] = j0[4*i] ^ byte(w>>24)
		out[4*i+1] = j0[4*i+1] ^ byte(w>>16)
		out[4*i+2] = j0[4*i+2] ^ byte(w>>8)
		out[4*i+3] = j0[4*i+3] ^ byte(w)
	}
	clear(y[:])
	clear(j0[:])
}

// load reads a block as big-endian words, with the last word first.
func load(b *[BlockSize]byte) [4]uint32 {
	var w [4]uint32
	for i := range 4 {
		w[3-i] = uint32(b[4*i])<<24 | uint32(b[4*i+1])<<16 | uint32(b[4*i+2])<<8 | uint32(b[4*i+3])
	}
	return w
}

// ghash adds b, padded with zeros to whole blocks, to y.
func (g *GCM) ghash(y *[4]uint32, b []byte) {
	for len(b) > 0 {
		var blk [BlockSize]byte
		n := copy(blk[:], b)
		x := load(&blk)
		for i := range y {
			y[i] ^= x[i]
		}
		g.mul(y)
		b = b[n:]
	}
}

// mul sets y = y * H in GF(2^128) (section 6.3). It is the constant-time
// multiplication of BearSSL ghash_ctmul.c, without tables.
func (g *GCM) mul(y *[4]uint32) {
	a, b := y, &g.h
	// The carry-less product with Karatsuba: three 64x64 products.
	var lo, hi, mid [4]uint32
	clmul64(&lo, a[1], a[0], b[1], b[0])
	clmul64(&hi, a[3], a[2], b[3], b[2])
	clmul64(&mid, a[1]^a[3], a[0]^a[2], b[1]^b[3], b[0]^b[2])
	var z [8]uint32
	for i := range 4 {
		mid[i] ^= lo[i] ^ hi[i]
		z[i] ^= lo[i]
		z[i+2] ^= mid[i]
		z[i+4] ^= hi[i]
	}
	// GCM keeps the bits in reverse order, so the 255-bit product moves up
	// by one bit. Then reduce modulo x^128 + x^7 + x^2 + x + 1.
	for i := 7; i > 0; i-- {
		z[i] = z[i]<<1 | z[i-1]>>31
	}
	z[0] <<= 1
	for i := range 4 {
		w := z[i]
		z[i+4] ^= w ^ w>>1 ^ w>>2 ^ w>>7
		z[i+3] ^= w<<31 ^ w<<30 ^ w<<25
	}
	copy(y[:], z[4:])
}

// clmul64 sets z to the carry-less product of (x1, x0) and (y1, y0), with
// the high words first.
func clmul64(z *[4]uint32, x1, x0, y1, y0 uint32) {
	l1, l0 := bmul(x0, y0)
	h1, h0 := bmul(x1, y1)
	m1, m0 := bmul(x0^x1, y0^y1)
	m0 ^= l0 ^ h0
	m1 ^= l1 ^ h1
	z[0], z[1], z[2], z[3] = l0, l1^m0, h0^m1, h1
}

// bmul is the carry-less 32x32 product, from integer products of the bits
// with gaps of 3 bits, so the carries do not reach the next bit.
func bmul(x, y uint32) (hi, lo uint32) {
	x0, x1, x2, x3 := uint64(x&0x11111111), uint64(x&0x22222222), uint64(x&0x44444444), uint64(x&0x88888888)
	y0, y1, y2, y3 := uint64(y&0x11111111), uint64(y&0x22222222), uint64(y&0x44444444), uint64(y&0x88888888)
	z0 := x0*y0 ^ x1*y3 ^ x2*y2 ^ x3*y1
	z1 := x0*y1 ^ x1*y0 ^ x2*y3 ^ x3*y2
	z2 := x0*y2 ^ x1*y1 ^ x2*y0 ^ x3*y3
	z3 := x0*y3 ^ x1*y2 ^ x2*y1 ^ x3*y0
	z := z0&0x1111111111111111 | z1&0x2222222222222222 | z2&0x4444444444444444 | z3&0x8888888888888888
	return uint32(z >> 32), uint32(z)
}
