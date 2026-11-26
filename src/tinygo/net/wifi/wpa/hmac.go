package wpa

import (
	"encoding/binary"
	"unsafe"

	"tinygo/crypto/sha1"
)

// mac is HMAC-SHA1 (RFC 2104) with keys of at most one SHA-1 block. It keeps
// the states after the ipad and opad blocks, so each sum hashes only the
// message, and it does not call the GC heap. All WPA2-Personal keys fit in
// one block.
type mac struct {
	inner, outer sha1.State
}

func (m *mac) init(key []byte) {
	var ipad, opad [sha1.BlockSize]byte
	for i := range ipad {
		var b byte
		if i < len(key) {
			b = key[i]
		}
		ipad[i] = b ^ 0x36
		opad[i] = b ^ 0x5c
	}
	m.inner, m.outer = sha1.Init, sha1.Init
	sha1.Block(&m.inner, &ipad)
	sha1.Block(&m.outer, &opad)
	clear(ipad[:])
	clear(opad[:])
}

func (m *mac) wipe() {
	clear(m.inner[:])
	clear(m.outer[:])
}

// sum returns HMAC(key, a || b || c || d).
func (m *mac) sum(a, b, c, d []byte) [sha1.Size]byte {
	var h sha1.Digest
	var inner, out [sha1.Size]byte
	h.Reset(m.inner, sha1.BlockSize)
	h.Write(a)
	h.Write(b)
	h.Write(c)
	h.Write(d)
	h.Sum(&inner)
	h.Reset(m.outer, sha1.BlockSize)
	h.Write(inner[:])
	h.Sum(&out)
	clear(inner[:])
	return out
}

// pbkdf2SHA1 fills out with PBKDF2-HMAC-SHA1 (RFC 8018 section 5.2).
func pbkdf2SHA1(password, salt []byte, iterations int, out []byte) {
	var m mac
	m.init(password)
	var index [4]byte
	for block := uint32(1); len(out) > 0; block++ {
		binary.BigEndian.PutUint32(index[:], block)
		u := m.sum(salt, index[:], nil, nil)
		t := u
		// The inner and the outer message are 20 bytes after the key block,
		// so one padded block serves both (FIPS 180-4 section 5.1.1).
		var blk [sha1.BlockSize]byte
		blk[sha1.Size] = 0x80
		binary.BigEndian.PutUint64(blk[sha1.BlockSize-8:], (sha1.BlockSize+sha1.Size)*8)
		for i := 1; i < iterations; i++ {
			s := m.inner
			copy(blk[:], u[:])
			sha1.Block(&s, &blk)
			copy(blk[:], s[:])
			s = m.outer
			sha1.Block(&s, &blk)
			u = s
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = out[copy(out, t[:]):]
		clear(u[:])
		clear(t[:])
		clear(blk[:])
	}
	m.wipe()
}

// prf fills out with the IEEE 802.11 PRF, HMAC-SHA1(K, A || 0 || B || i) for i = 0, 1, ...
// (IEEE 802.11-2020 12.7.1.2).
func prf(key []byte, label string, data, out []byte) {
	var m mac
	m.init(key)
	a := unsafe.Slice(unsafe.StringData(label), len(label))
	zero := [1]byte{0}
	var index [1]byte
	for ; len(out) > 0; index[0]++ {
		sum := m.sum(a, zero[:], data, index[:])
		out = out[copy(out, sum[:]):]
		clear(sum[:])
	}
	m.wipe()
}
