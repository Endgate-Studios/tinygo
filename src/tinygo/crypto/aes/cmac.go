package aes

// CMAC writes the AES-CMAC of msg (RFC 4493) to out.
func (c *Cipher) CMAC(out *[BlockSize]byte, msg []byte) error {
	if c.rk == nil {
		return ErrClosed
	}
	// The subkeys K1 and K2 of RFC 4493 section 2.3.
	var k1, k2 [BlockSize]byte
	c.encrypt(&k1, &k1)
	shiftXor(&k1, &k1)
	shiftXor(&k2, &k1)
	var x, last [BlockSize]byte
	n := (len(msg) + BlockSize - 1) / BlockSize
	if n == 0 {
		n = 1
	}
	for i := 0; i < n-1; i++ {
		for j := range x {
			x[j] ^= msg[BlockSize*i+j]
		}
		c.encrypt(&x, &x)
	}
	rest := msg[BlockSize*(n-1):]
	copy(last[:], rest)
	if len(rest) == BlockSize {
		for j := range last {
			last[j] ^= k1[j]
		}
	} else {
		last[len(rest)] = 0x80
		for j := range last {
			last[j] ^= k2[j]
		}
	}
	for j := range x {
		x[j] ^= last[j]
	}
	c.encrypt(out, &x)
	clear(k1[:])
	clear(k2[:])
	clear(x[:])
	clear(last[:])
	return nil
}

// shiftXor sets dst = src << 1, with 0x87 into the last byte when the top bit
// of src was set.
func shiftXor(dst, src *[BlockSize]byte) {
	msb := src[0] >> 7
	for i := 0; i < BlockSize-1; i++ {
		dst[i] = src[i]<<1 | src[i+1]>>7
	}
	dst[BlockSize-1] = src[BlockSize-1]<<1 ^ 0x87&-msb
}
