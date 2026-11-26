// Package smp is LE pairing (Core spec v5.4 Vol 3 Part H). The crypto functions take values in the spec order,
// most significant byte first, and take their AES round keys from an injected allocator that can free.
package smp

import (
	"runtime/memory"
	"tinygo/crypto/aes"
)

// Memory is where the AES round keys come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilMemory Error = "smp: memory cannot be nil"
	ErrNoMemory  Error = "smp: out of memory"
)

// cmac is AES-CMAC (RFC 4493) that takes the message in parts. The last block waits in buf
// until Sum, because only the last block is mixed with K1 or K2.
type cmac struct {
	c   aes.Cipher
	x   [16]byte // chaining value
	buf [16]byte
	n   int
}

func (m *cmac) init(mem Memory, key *[16]byte) error {
	if mem == nil {
		return ErrNilMemory
	}
	if err := m.c.Init(mem, key[:]); err != nil {
		return ErrNoMemory
	}
	clear(m.x[:])
	m.n = 0
	return nil
}

func (m *cmac) write(p []byte) {
	for len(p) > 0 {
		if m.n == 16 {
			for i := range m.x {
				m.x[i] ^= m.buf[i]
			}
			m.c.Encrypt(&m.x, &m.x)
			m.n = 0
		}
		k := copy(m.buf[m.n:], p)
		m.n += k
		p = p[k:]
	}
}

// sum finishes the MAC (RFC 4493 section 2.4), then zeroes and frees the round keys.
func (m *cmac) sum(out *[16]byte) {
	var l, k [16]byte
	m.c.Encrypt(&l, &l)
	subkey(&l, &k) // K1
	if m.n < 16 {
		subkey(&k, &k) // K2
		m.buf[m.n] = 0x80
		clear(m.buf[m.n+1:])
	}
	for i := range m.x {
		m.x[i] ^= m.buf[i] ^ k[i]
	}
	m.c.Encrypt(out, &m.x)
	clear(l[:])
	clear(k[:])
	clear(m.x[:])
	clear(m.buf[:])
	m.c.Close()
}

// subkey doubles in in GF(2^128) (RFC 4493 section 2.3).
func subkey(in, out *[16]byte) {
	carry := in[0] >> 7
	for i := 0; i < 15; i++ {
		out[i] = in[i]<<1 | in[i+1]>>7
	}
	out[15] = in[15] << 1
	if carry != 0 {
		out[15] ^= 0x87
	}
}

// CMAC computes AES-CMAC of msg under key (RFC 4493).
func CMAC(mem Memory, key *[16]byte, msg []byte, out *[16]byte) error {
	var m cmac
	if err := m.init(mem, key); err != nil {
		return err
	}
	m.write(msg)
	m.sum(out)
	return nil
}
