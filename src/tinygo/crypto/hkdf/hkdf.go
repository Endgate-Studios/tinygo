// Package hkdf is HKDF with HMAC-SHA256 (RFC 5869). It does not allocate.
package hkdf

import "tinygo/crypto/sha256"

// Size is the length of a pseudorandom key.
const Size = sha256.Size

// Extract sets prk = HMAC-SHA256(salt, ikm). An empty salt is Size zeros.
func Extract(prk *[Size]byte, salt, ikm []byte) {
	var zero [Size]byte
	if len(salt) == 0 {
		salt = zero[:]
	}
	var h sha256.HMAC
	h.Init(salt)
	h.Sum(prk, ikm, nil, nil, nil)
	h.Clear()
}

// Expand fills out, of at most 255 * Size bytes, from prk and the info
// info1 || info2.
func Expand(out, prk, info1, info2 []byte) {
	var h sha256.HMAC
	h.Init(prk)
	var t [Size]byte
	prev := t[:0]
	for i := byte(1); len(out) > 0; i++ {
		c := [1]byte{i}
		h.Sum(&t, prev, info1, info2, c[:])
		prev = t[:]
		out = out[copy(out, t[:]):]
	}
	clear(t[:])
	h.Clear()
}
