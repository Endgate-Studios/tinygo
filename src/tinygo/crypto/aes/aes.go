// Package aes is the AES block cipher (FIPS 197) with round keys in memory from an injected allocator.
// Encrypt and Decrypt work on the stack and do not call the GC heap. Close zeroes and frees the round keys.
package aes

import "runtime/memory"

// BlockSize is the AES block size in bytes.
const BlockSize = 16

// Memory is where a Cipher allocates its round keys.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilMemory Error = "aes: memory cannot be nil"
	ErrNoMemory  Error = "aes: out of memory"
	ErrKeySize   Error = "aes: key must be 16, 24 or 32 bytes"
	ErrClosed    Error = "aes: cipher is closed"
)

// Cipher encrypts and decrypts single blocks with one key.
type Cipher struct {
	mem    Memory
	rk     []byte // round keys, 16 bytes per round plus one
	rounds int
}

// New returns a Cipher on the GC heap with round keys from mem.
// Use Init to keep the Cipher on the stack or in another struct.
func New(mem Memory, key []byte) (*Cipher, error) {
	c := new(Cipher)
	if err := c.Init(mem, key); err != nil {
		return nil, err
	}
	return c, nil
}

// Init expands key into round keys allocated from mem. Close frees them.
func (c *Cipher) Init(mem Memory, key []byte) error {
	if mem == nil {
		return ErrNilMemory
	}
	var rounds int
	switch len(key) {
	case 16:
		rounds = 10
	case 24:
		rounds = 12
	case 32:
		rounds = 14
	default:
		return ErrKeySize
	}
	rk := memory.MakeSlice[byte](mem, BlockSize*(rounds+1))
	if rk == nil {
		return ErrNoMemory
	}
	expandKey(key, rk)
	c.mem, c.rk, c.rounds = mem, rk, rounds
	return nil
}

// Close zeroes and frees the round keys. The Cipher cannot be used after Close.
func (c *Cipher) Close() {
	if c.rk == nil {
		return
	}
	clear(c.rk)
	memory.FreeSlice(c.mem, c.rk)
	c.rk = nil
}

// Encrypt encrypts src into dst. dst and src can be the same block.
func (c *Cipher) Encrypt(dst, src *[BlockSize]byte) error {
	if c.rk == nil {
		return ErrClosed
	}
	c.encrypt(dst, src)
	return nil
}

// Decrypt decrypts src into dst. dst and src can be the same block.
func (c *Cipher) Decrypt(dst, src *[BlockSize]byte) error {
	if c.rk == nil {
		return ErrClosed
	}
	c.decrypt(dst, src)
	return nil
}

// key gives the cipher key, which is the start of the round keys (FIPS 197
// section 5.2).
func (c *Cipher) key() []byte { return c.rk[:4*(c.rounds-6)] }

func (c *Cipher) encryptSoft(dst, src *[BlockSize]byte) {
	s := *src
	addRoundKey(&s, c.rk[0:])
	for r := 1; r < c.rounds; r++ {
		subBytes(&s)
		shiftRows(&s)
		mixColumns(&s)
		addRoundKey(&s, c.rk[BlockSize*r:])
	}
	subBytes(&s)
	shiftRows(&s)
	addRoundKey(&s, c.rk[BlockSize*c.rounds:])
	*dst = s
	clear(s[:])
}

func (c *Cipher) decryptSoft(dst, src *[BlockSize]byte) {
	s := *src
	addRoundKey(&s, c.rk[BlockSize*c.rounds:])
	for r := c.rounds - 1; r > 0; r-- {
		invShiftRows(&s)
		invSubBytes(&s)
		addRoundKey(&s, c.rk[BlockSize*r:])
		invMixColumns(&s)
	}
	invShiftRows(&s)
	invSubBytes(&s)
	addRoundKey(&s, c.rk[0:])
	*dst = s
	clear(s[:])
}

// Closed reports whether the Cipher has no round keys, before Init or after Close.
func (c *Cipher) Closed() bool { return c.rk == nil }
