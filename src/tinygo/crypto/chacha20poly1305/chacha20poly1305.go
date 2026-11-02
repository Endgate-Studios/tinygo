// Package chacha20poly1305 is the ChaCha20-Poly1305 AEAD of RFC 8439 section
// 2.8, with a 12-byte nonce. Seal and Open do not allocate when dst has the
// capacity, and they work in place. The tag check is constant time.
package chacha20poly1305

import (
	"crypto/subtle"
	"encoding/binary"
	"unsafe"
)

const (
	// KeySize is the size of a key in bytes.
	KeySize = 32
	// NonceSize is the size of a nonce in bytes.
	NonceSize = 12
	// Overhead is the size of the tag that Seal appends, in bytes.
	Overhead = 16
)

// maxText is the longest plaintext, 2^32 - 1 blocks of 64 bytes after the
// block of the Poly1305 key (RFC 8439 section 2.8).
const maxText = (1<<32-1)*64 - 64

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

// Error gives the message.
func (e Error) Error() string { return string(e) }

const (
	// ErrKeySize tells that a key is not KeySize bytes.
	ErrKeySize Error = "chacha20poly1305: key must be 32 bytes"
	// ErrNonceSize tells that a nonce is not NonceSize bytes.
	ErrNonceSize Error = "chacha20poly1305: nonce must be 12 bytes"
	// ErrTooLarge tells that a plaintext is longer than RFC 8439 allows.
	ErrTooLarge Error = "chacha20poly1305: plaintext too large"
	// ErrOverlap tells that dst overlaps the input other than in place.
	ErrOverlap Error = "chacha20poly1305: dst overlaps the input but not at the same byte"
	// ErrAuth tells that the tag does not match.
	ErrAuth Error = "chacha20poly1305: message authentication failed"
)

// AEAD is ChaCha20-Poly1305 with one key. Use New, or Init to keep it in
// another struct. It implements crypto/cipher.AEAD.
type AEAD struct {
	key [8]uint32
}

// New returns an AEAD with key, which is KeySize bytes.
func New(key []byte) (*AEAD, error) {
	a := new(AEAD)
	if err := a.Init(key); err != nil {
		return nil, err
	}
	return a, nil
}

// Init sets the key, which is KeySize bytes.
func (a *AEAD) Init(key []byte) error {
	if len(key) != KeySize {
		return ErrKeySize
	}
	for i := range a.key {
		a.key[i] = binary.LittleEndian.Uint32(key[4*i:])
	}
	return nil
}

// Clear zeroes the key.
func (a *AEAD) Clear() { clear(a.key[:]) }

// NonceSize gives the size of a nonce, NonceSize.
func (a *AEAD) NonceSize() int { return NonceSize }

// Overhead gives the size of the tag, Overhead.
func (a *AEAD) Overhead() int { return Overhead }

// Seal encrypts and authenticates plaintext and additionalData, appends the
// ciphertext and the tag to dst and gives the result. To encrypt in place,
// pass plaintext[:0] as dst; otherwise dst must not overlap plaintext. It
// panics with ErrNonceSize, ErrTooLarge or ErrOverlap.
func (a *AEAD) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != NonceSize {
		panic(ErrNonceSize)
	}
	if uint64(len(plaintext)) > maxText {
		panic(ErrTooLarge)
	}
	ret, out := grow(dst, len(plaintext)+Overhead)
	if inexactOverlap(out, plaintext) {
		panic(ErrOverlap)
	}
	var s state
	a.setup(&s, nonce)
	ct := out[:len(plaintext)]
	s.xor(ct, plaintext)
	var tag [Overhead]byte
	s.mac.tag(&tag, additionalData, ct)
	copy(out[len(plaintext):], tag[:])
	s.clear()
	return ret
}

// Open authenticates ciphertext, which ends with the tag, and
// additionalData, decrypts it, appends the plaintext to dst and gives the
// result. To decrypt in place, pass ciphertext[:0] as dst; otherwise dst must
// not overlap ciphertext. It gives ErrNonceSize, ErrOverlap or ErrAuth, and
// then writes nothing.
func (a *AEAD) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		return nil, ErrNonceSize
	}
	if len(ciphertext) < Overhead || uint64(len(ciphertext)) > maxText+Overhead {
		return nil, ErrAuth
	}
	ct := ciphertext[:len(ciphertext)-Overhead]
	ret, out := grow(dst, len(ct))
	if inexactOverlap(out, ct) {
		return nil, ErrOverlap
	}
	var s state
	a.setup(&s, nonce)
	var tag [Overhead]byte
	s.mac.tag(&tag, additionalData, ct)
	ok := subtle.ConstantTimeCompare(tag[:], ciphertext[len(ct):]) == 1
	clear(tag[:])
	if !ok {
		s.clear()
		return nil, ErrAuth
	}
	s.xor(out, ct)
	s.clear()
	return ret, nil
}

// state is the ChaCha20 input block and the Poly1305 state of one message.
type state struct {
	in  [16]uint32
	mac poly1305
}

// setup sets the ChaCha20 input for the nonce, and the Poly1305 key from the
// first 32 bytes of block 0 (RFC 8439 section 2.6). The counter is then 1.
func (a *AEAD) setup(s *state, nonce []byte) {
	s.in = [16]uint32{
		0x61707865, 0x3320646e, 0x79622d32, 0x6b206574, // "expand 32-byte k"
		a.key[0], a.key[1], a.key[2], a.key[3], a.key[4], a.key[5], a.key[6], a.key[7],
		0, binary.LittleEndian.Uint32(nonce), binary.LittleEndian.Uint32(nonce[4:]), binary.LittleEndian.Uint32(nonce[8:]),
	}
	var ks [16]uint32
	block(&ks, &s.in)
	s.mac.init(&ks)
	clear(ks[:])
	s.in[12] = 1
}

func (s *state) clear() {
	clear(s.in[:])
	s.mac = poly1305{}
}

// xor sets dst to src XOR the key stream from the counter in s.in[12]. dst
// and src have the same length and are the same bytes or do not overlap.
func (s *state) xor(dst, src []byte) {
	var ks [16]uint32
	for len(src) >= 64 {
		block(&ks, &s.in)
		s.in[12]++
		for i, k := range ks {
			v := binary.LittleEndian.Uint32(src[4*i:]) ^ k
			binary.LittleEndian.PutUint32(dst[4*i:], v)
		}
		dst, src = dst[64:], src[64:]
	}
	if len(src) > 0 {
		block(&ks, &s.in)
		s.in[12]++
		for i := range src {
			dst[i] = src[i] ^ byte(ks[i/4]>>(8*(i%4)))
		}
	}
	clear(ks[:])
}

// block sets out to the ChaCha20 block of in (RFC 8439 section 2.3).
func block(out, in *[16]uint32) {
	x0, x1, x2, x3, x4, x5, x6, x7 := in[0], in[1], in[2], in[3], in[4], in[5], in[6], in[7]
	x8, x9, x10, x11, x12, x13, x14, x15 := in[8], in[9], in[10], in[11], in[12], in[13], in[14], in[15]
	for i := 0; i < 10; i++ {
		x0, x4, x8, x12 = quarter(x0, x4, x8, x12)
		x1, x5, x9, x13 = quarter(x1, x5, x9, x13)
		x2, x6, x10, x14 = quarter(x2, x6, x10, x14)
		x3, x7, x11, x15 = quarter(x3, x7, x11, x15)
		x0, x5, x10, x15 = quarter(x0, x5, x10, x15)
		x1, x6, x11, x12 = quarter(x1, x6, x11, x12)
		x2, x7, x8, x13 = quarter(x2, x7, x8, x13)
		x3, x4, x9, x14 = quarter(x3, x4, x9, x14)
	}
	out[0], out[1], out[2], out[3] = x0+in[0], x1+in[1], x2+in[2], x3+in[3]
	out[4], out[5], out[6], out[7] = x4+in[4], x5+in[5], x6+in[6], x7+in[7]
	out[8], out[9], out[10], out[11] = x8+in[8], x9+in[9], x10+in[10], x11+in[11]
	out[12], out[13], out[14], out[15] = x12+in[12], x13+in[13], x14+in[14], x15+in[15]
}

// quarter is the quarter round of RFC 8439 section 2.1.
func quarter(a, b, c, d uint32) (uint32, uint32, uint32, uint32) {
	a += b
	d ^= a
	d = d<<16 | d>>16
	c += d
	b ^= c
	b = b<<12 | b>>20
	a += b
	d ^= a
	d = d<<8 | d>>24
	c += d
	b ^= c
	b = b<<7 | b>>25
	return a, b, c, d
}

// grow extends dst by n bytes, in place when it has the capacity, and gives
// the whole slice and the n new bytes.
func grow(dst []byte, n int) (whole, tail []byte) {
	total := len(dst) + n
	if cap(dst) >= total {
		whole = dst[:total]
	} else {
		whole = make([]byte, total)
		copy(whole, dst)
	}
	return whole, whole[len(dst):]
}

// inexactOverlap reports whether x and y share memory but do not start at the
// same byte.
func inexactOverlap(x, y []byte) bool {
	if len(x) == 0 || len(y) == 0 || &x[0] == &y[0] {
		return false
	}
	x0, x1 := uintptr(unsafe.Pointer(&x[0])), uintptr(unsafe.Pointer(&x[len(x)-1]))
	y0, y1 := uintptr(unsafe.Pointer(&y[0])), uintptr(unsafe.Pointer(&y[len(y)-1]))
	return x0 <= y1 && y0 <= x1
}
