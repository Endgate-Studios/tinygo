package wpa

import (
	"encoding/binary"

	"tinygo/crypto/aes"
)

// keyWrapIV is the initial value of RFC 3394 section 2.2.3.1.
const keyWrapIV = 0xA6A6A6A6A6A6A6A6

// UnwrapKey decrypts an AES key wrap, for example the GTK in EAPOL-Key message 3
// (IEEE 802.11-2020 12.7.2, RFC 3394). out must hold len(wrapped)-8 bytes.
func (k *Keys) UnwrapKey(wrapped, out []byte) error {
	if k.closed {
		return ErrClosed
	}
	if k.ptk == nil {
		return ErrNoPTK
	}
	if len(wrapped)%8 != 0 || len(wrapped) < 24 || len(out) < len(wrapped)-8 {
		return ErrWrappedSize
	}
	var c aes.Cipher
	if err := c.Init(k.mem, k.KEK()); err != nil {
		return ErrNoMemory
	}
	ok := unwrap(&c, wrapped, out[:len(wrapped)-8])
	c.Close()
	if !ok {
		clear(out)
		return ErrUnwrap
	}
	return nil
}

// unwrap is the index based unwrap of RFC 3394 section 2.2.2. It reports whether the integrity check passed.
func unwrap(c *aes.Cipher, wrapped, out []byte) bool {
	n := len(out) / 8
	a := binary.BigEndian.Uint64(wrapped)
	copy(out, wrapped[8:])
	var b [aes.BlockSize]byte
	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			binary.BigEndian.PutUint64(b[:8], a^uint64(n*j+i))
			copy(b[8:], out[8*(i-1):8*i])
			c.Decrypt(&b, &b)
			a = binary.BigEndian.Uint64(b[:8])
			copy(out[8*(i-1):8*i], b[8:])
		}
	}
	clear(b[:])
	return a == keyWrapIV
}

// WrapKey encrypts plain with an AES key wrap under the KEK, for example the
// key data of EAPOL-Key message 3 (IEEE 802.11-2020 12.7.2, RFC 3394). plain
// is a multiple of 8 bytes and at least 16. out must hold len(plain)+8 bytes.
func (k *Keys) WrapKey(plain, out []byte) error {
	if k.closed {
		return ErrClosed
	}
	if k.ptk == nil {
		return ErrNoPTK
	}
	if len(plain)%8 != 0 || len(plain) < 16 || len(out) < len(plain)+8 {
		return ErrWrappedSize
	}
	var c aes.Cipher
	if err := c.Init(k.mem, k.KEK()); err != nil {
		return ErrNoMemory
	}
	wrap(&c, plain, out[:len(plain)+8])
	c.Close()
	return nil
}

// wrap is the index based wrap of RFC 3394 section 2.2.1.
func wrap(c *aes.Cipher, plain, out []byte) {
	n := len(plain) / 8
	a := uint64(keyWrapIV)
	copy(out[8:], plain)
	var b [aes.BlockSize]byte
	for j := 0; j <= 5; j++ {
		for i := 1; i <= n; i++ {
			binary.BigEndian.PutUint64(b[:8], a)
			copy(b[8:], out[8*i:8*i+8])
			c.Encrypt(&b, &b) // the key size is checked in Init
			a = binary.BigEndian.Uint64(b[:8]) ^ uint64(n*j+i)
			copy(out[8*i:8*i+8], b[8:])
		}
	}
	binary.BigEndian.PutUint64(out, a)
	clear(b[:])
}
