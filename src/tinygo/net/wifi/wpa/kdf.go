package wpa

import (
	"unsafe"

	"tinygo/crypto/sha256"
)

// KDFSHA256 fills out with KDF-SHA256 of IEEE 802.11-2020 12.7.1.6.2:
// HMAC-SHA256(key, i || label || context || length) for i = 1, 2, ..., with i
// and the length in bits as 16-bit little-endian values.
func KDFSHA256(key []byte, label string, context, out []byte) {
	var h sha256.HMAC
	h.Init(key)
	bits := uint16(len(out) * 8)
	length := [2]byte{byte(bits), byte(bits >> 8)}
	l := unsafe.Slice(unsafe.StringData(label), len(label))
	var sum [sha256.Size]byte
	for i := uint16(1); len(out) > 0; i++ {
		counter := [2]byte{byte(i), byte(i >> 8)}
		h.Sum(&sum, counter[:], l, context, length[:])
		out = out[copy(out, sum[:]):]
	}
	clear(sum[:])
	h.Clear()
}
