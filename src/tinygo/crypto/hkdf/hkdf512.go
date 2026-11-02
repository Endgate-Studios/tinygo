package hkdf

import "tinygo/crypto/sha512"

// Size512 is the length of a pseudorandom key of HKDF-SHA512.
const Size512 = sha512.Size

// zeros512 is the empty salt of HKDF-SHA512.
var zeros512 [Size512]byte

// Extract512 sets prk = HMAC-SHA512(salt, ikm). An empty salt is Size512
// zeros. HomeKit pairing, and so AirPlay 2, derives its keys this way.
func Extract512(prk *[Size512]byte, salt, ikm []byte) {
	if len(salt) == 0 {
		salt = zeros512[:]
	}
	var h sha512.HMAC
	h.Init(salt)
	h.Sum(prk, ikm, nil, nil, nil)
	h.Clear()
}

// Expand512 fills out, of at most 255 * Size512 bytes, from prk and the info
// info1 || info2.
func Expand512(out, prk, info1, info2 []byte) {
	var h sha512.HMAC
	h.Init(prk)
	var t [Size512]byte
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

// Key512 is Extract512 then Expand512: out is derived from ikm with salt
// and info, as HomeKit's "Control-Salt" and "Pair-Verify-Encrypt-Salt" keys
// are. It runs once per pairing, not per packet: under TinyGo, Expand512's
// block and counter escape to the heap, two small allocations a call.
func Key512(out, salt, ikm, info []byte) {
	var prk [Size512]byte
	Extract512(&prk, salt, ikm)
	Expand512(out, prk[:], info, nil)
	clear(prk[:])
}
