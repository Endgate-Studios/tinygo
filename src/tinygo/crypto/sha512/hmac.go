package sha512

// HMAC is HMAC-SHA512 (RFC 2104). It keeps the states after the ipad and opad
// blocks, so each Sum hashes only the message.
type HMAC struct {
	inner, outer State
}

// Init sets the key. A key longer than one block is hashed first.
func (h *HMAC) Init(key []byte) {
	var k [Size]byte
	if len(key) > BlockSize {
		var d Digest
		d.Reset(Init512, 0)
		d.Write(key)
		d.Sum(&k)
		key = k[:]
	}
	var ipad, opad [BlockSize]byte
	for i := range ipad {
		var b byte
		if i < len(key) {
			b = key[i]
		}
		ipad[i] = b ^ 0x36
		opad[i] = b ^ 0x5c
	}
	h.inner, h.outer = Init512, Init512
	Block(&h.inner, &ipad)
	Block(&h.outer, &opad)
	clear(ipad[:])
	clear(opad[:])
	clear(k[:])
}

// Sum writes HMAC(key, a || b || c || d) to out.
func (h *HMAC) Sum(out *[Size]byte, a, b, c, d []byte) {
	var dg Digest
	var inner [Size]byte
	dg.Reset(h.inner, BlockSize)
	dg.Write(a)
	dg.Write(b)
	dg.Write(c)
	dg.Write(d)
	dg.Sum(&inner)
	dg.Reset(h.outer, BlockSize)
	dg.Write(inner[:])
	dg.Sum(out)
	clear(inner[:])
}

// Clear zeroes the key states.
func (h *HMAC) Clear() {
	clear(h.inner[:])
	clear(h.outer[:])
}
