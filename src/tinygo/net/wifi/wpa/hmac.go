package wpa

import (
	"crypto/sha1"
	"encoding/binary"
	"unsafe"
)

// mac is HMAC-SHA1 (RFC 2104) with keys of at most one SHA-1 block. It hashes from a
// caller buffer and does not call the GC heap. All WPA2-Personal keys fit in one block.
type mac struct {
	ipad, opad [sha1.BlockSize]byte
}

func (m *mac) init(key []byte) {
	for i := range m.ipad {
		var b byte
		if i < len(key) {
			b = key[i]
		}
		m.ipad[i] = b ^ 0x36
		m.opad[i] = b ^ 0x5c
	}
}

func (m *mac) wipe() {
	clear(m.ipad[:])
	clear(m.opad[:])
}

// sum returns HMAC(key, a || b || c || d). scratch must hold one block plus the message.
func (m *mac) sum(scratch, a, b, c, d []byte) [sha1.Size]byte {
	n := copy(scratch, m.ipad[:])
	n += copy(scratch[n:], a)
	n += copy(scratch[n:], b)
	n += copy(scratch[n:], c)
	n += copy(scratch[n:], d)
	inner := sha1.Sum(scratch[:n])
	n = copy(scratch, m.opad[:])
	n += copy(scratch[n:], inner[:])
	clear(inner[:])
	return sha1.Sum(scratch[:n])
}

// pbkdf2SHA1 fills out with PBKDF2-HMAC-SHA1 (RFC 8018 section 5.2).
func pbkdf2SHA1(scratch, password, salt []byte, iterations int, out []byte) {
	var m mac
	m.init(password)
	var index [4]byte
	for block := uint32(1); len(out) > 0; block++ {
		binary.BigEndian.PutUint32(index[:], block)
		u := m.sum(scratch, salt, index[:], nil, nil)
		t := u
		for i := 1; i < iterations; i++ {
			u = m.sum(scratch, u[:], nil, nil, nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = out[copy(out, t[:]):]
		clear(u[:])
		clear(t[:])
	}
	m.wipe()
}

// prf fills out with the IEEE 802.11 PRF, HMAC-SHA1(K, A || 0 || B || i) for i = 0, 1, ...
// (IEEE 802.11-2020 12.7.1.2).
func prf(scratch, key []byte, label string, data, out []byte) {
	var m mac
	m.init(key)
	a := unsafe.Slice(unsafe.StringData(label), len(label))
	zero := [1]byte{0}
	var index [1]byte
	for ; len(out) > 0; index[0]++ {
		sum := m.sum(scratch, a, zero[:], data, index[:])
		out = out[copy(out, sum[:]):]
		clear(sum[:])
	}
	m.wipe()
}
