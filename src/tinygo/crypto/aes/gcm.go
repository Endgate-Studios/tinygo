package aes

import (
	"crypto/subtle"
	"encoding/binary"
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
	h [BlockSize]byte // the hash key, E(K, 0)
}

// Init sets the cipher. The GCM uses c until c is closed.
func (g *GCM) Init(c *Cipher) error {
	if c.rk == nil {
		return ErrClosed
	}
	g.c = c
	clear(g.h[:])
	c.encrypt(&g.h, &g.h)
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
	var cb, ks [BlockSize]byte
	copy(cb[:], nonce[:])
	counter := uint32(2)
	for len(src) > 0 {
		binary.BigEndian.PutUint32(cb[12:], counter)
		g.c.encrypt(&ks, &cb)
		n := min(len(src), BlockSize)
		for i := 0; i < n; i++ {
			dst[i] = src[i] ^ ks[i]
		}
		dst, src = dst[n:], src[n:]
		counter++
	}
	clear(ks[:])
}

// tag computes GHASH over aad and ct, and encrypts it with the block J0
// (section 7.1).
func (g *GCM) tag(out *[TagSize]byte, nonce *[NonceSize]byte, aad, ct []byte) {
	var y [BlockSize]byte
	g.ghash(&y, aad)
	g.ghash(&y, ct)
	var lens [BlockSize]byte
	binary.BigEndian.PutUint64(lens[:8], uint64(len(aad))*8)
	binary.BigEndian.PutUint64(lens[8:], uint64(len(ct))*8)
	g.ghashBlock(&y, &lens)
	var j0 [BlockSize]byte
	copy(j0[:], nonce[:])
	j0[15] = 1
	g.c.encrypt(&j0, &j0)
	for i := range out {
		out[i] = y[i] ^ j0[i]
	}
	clear(y[:])
	clear(j0[:])
}

// ghash adds b, padded with zeros to whole blocks, to y.
func (g *GCM) ghash(y *[BlockSize]byte, b []byte) {
	for len(b) > 0 {
		var blk [BlockSize]byte
		n := copy(blk[:], b)
		g.ghashBlock(y, &blk)
		b = b[n:]
	}
}

// ghashBlock sets y = (y ^ x) * H in GF(2^128) (section 6.3), with masks
// in place of branches on the data.
func (g *GCM) ghashBlock(y, x *[BlockSize]byte) {
	vh := binary.BigEndian.Uint64(g.h[:8])
	vl := binary.BigEndian.Uint64(g.h[8:])
	xh := binary.BigEndian.Uint64(y[:8]) ^ binary.BigEndian.Uint64(x[:8])
	xl := binary.BigEndian.Uint64(y[8:]) ^ binary.BigEndian.Uint64(x[8:])
	var zh, zl uint64
	for i := 0; i < 128; i++ {
		var bit uint64
		if i < 64 {
			bit = xh >> (63 - i) & 1
		} else {
			bit = xl >> (127 - i) & 1
		}
		mask := -bit
		zh ^= vh & mask
		zl ^= vl & mask
		// V = V >> 1, with R = 0xe1 || 0^120 when the low bit was 1.
		r := -(vl & 1)
		vl = vl>>1 | vh<<63
		vh = vh>>1 ^ 0xe100000000000000&r
	}
	binary.BigEndian.PutUint64(y[:8], zh)
	binary.BigEndian.PutUint64(y[8:], zl)
}
