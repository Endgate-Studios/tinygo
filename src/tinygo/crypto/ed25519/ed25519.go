// Package ed25519 is the Ed25519 signature scheme of RFC 8032, with the API
// of crypto/ed25519. The field arithmetic is
// tinygo/crypto/internal/field25519 and the scalars are
// tinygo/crypto/internal/mpi, so the RSA accelerator does the
// multiplications on the ESP32-S3. Signing is constant time. Verify,
// SignInto, NewKeyFromSeedInto and GenerateKeyInto do not allocate. The
// working memory is on the stack: a table of 1 KB to sign, two to verify.
package ed25519

import (
	"crypto/subtle"
	"io"

	"tinygo/crypto/sha512"
)

const (
	// PublicKeySize is the size of a public key in bytes.
	PublicKeySize = 32
	// PrivateKeySize is the size of a private key in bytes: the seed and
	// then the public key.
	PrivateKeySize = 64
	// SignatureSize is the size of a signature in bytes.
	SignatureSize = 64
	// SeedSize is the size of a seed, the private key of RFC 8032, in bytes.
	SeedSize = 32
)

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

// Error gives the message.
func (e Error) Error() string { return string(e) }

const (
	// ErrSeedSize tells that a seed is not SeedSize bytes.
	ErrSeedSize Error = "ed25519: seed must be 32 bytes"
	// ErrPrivateKeySize tells that a private key is not PrivateKeySize bytes.
	ErrPrivateKeySize Error = "ed25519: private key must be 64 bytes"
	// ErrRandom tells that the random source is nil or failed.
	ErrRandom Error = "ed25519: random source failed"
)

// PublicKey is an Ed25519 public key, PublicKeySize bytes.
type PublicKey []byte

// PrivateKey is an Ed25519 private key, PrivateKeySize bytes: the seed and
// then the public key, as crypto/ed25519.
type PrivateKey []byte

// Equal reports whether pub and x are the same key.
func (pub PublicKey) Equal(x PublicKey) bool {
	return subtle.ConstantTimeCompare(pub, x) == 1
}

// Public gives the public key of priv. Unlike crypto/ed25519, it is not a
// copy: it shares the last 32 bytes of priv.
func (priv PrivateKey) Public() PublicKey {
	return PublicKey(priv[SeedSize:PrivateKeySize:PrivateKeySize])
}

// Seed gives the seed of priv. Unlike crypto/ed25519, it is not a copy: it
// shares the first 32 bytes of priv.
func (priv PrivateKey) Seed() []byte { return priv[:SeedSize:SeedSize] }

// Equal reports whether priv and x are the same key, in constant time.
func (priv PrivateKey) Equal(x PrivateKey) bool {
	return subtle.ConstantTimeCompare(priv, x) == 1
}

// GenerateKey draws a seed from rand and gives the key pair. The public key
// shares the private key's memory.
func GenerateKey(rand io.Reader) (PublicKey, PrivateKey, error) {
	priv := make(PrivateKey, PrivateKeySize)
	if err := GenerateKeyInto((*[PrivateKeySize]byte)(priv), rand); err != nil {
		return nil, nil, err
	}
	return priv.Public(), priv, nil
}

// GenerateKeyInto draws a seed from rand and writes the private key to
// priv. It gives ErrRandom when rand is nil or fails.
func GenerateKeyInto(priv *[PrivateKeySize]byte, rand io.Reader) error {
	if rand == nil {
		return ErrRandom
	}
	var seed [SeedSize]byte
	if _, err := io.ReadFull(rand, seed[:]); err != nil {
		clear(seed[:])
		return ErrRandom
	}
	NewKeyFromSeedInto(priv, &seed)
	clear(seed[:])
	return nil
}

// NewKeyFromSeed gives the private key of seed. It panics with ErrSeedSize
// when seed is not SeedSize bytes.
func NewKeyFromSeed(seed []byte) PrivateKey {
	if len(seed) != SeedSize {
		panic(ErrSeedSize)
	}
	priv := make(PrivateKey, PrivateKeySize)
	NewKeyFromSeedInto((*[PrivateKeySize]byte)(priv), (*[SeedSize]byte)(seed))
	return priv
}

// NewKeyFromSeedInto writes the private key of seed to priv (RFC 8032
// section 5.1.5). seed can be the first half of priv.
func NewKeyFromSeedInto(priv *[PrivateKeySize]byte, seed *[SeedSize]byte) {
	var h [sha512.Size]byte
	var a scalar
	var A point
	var pub [PublicKeySize]byte
	hash(&h, seed[:], nil, nil)
	secretScalar(&a, &h)
	scalarMultBase(&A, &a)
	A.bytes(&pub)
	copy(priv[:SeedSize], seed[:])
	copy(priv[SeedSize:], pub[:])
	clear(h[:])
	a, A = scalar{}, point{}
}

// Sign signs message with priv and gives the signature. It panics with
// ErrPrivateKeySize when priv is not PrivateKeySize bytes.
func Sign(priv PrivateKey, message []byte) []byte {
	sig := make([]byte, SignatureSize)
	SignInto((*[SignatureSize]byte)(sig), priv, message)
	return sig
}

// SignInto signs message with priv into sig (RFC 8032 section 5.1.6), in
// constant time. sig must not overlap message. It panics with
// ErrPrivateKeySize when priv is not PrivateKeySize bytes.
func SignInto(sig *[SignatureSize]byte, priv PrivateKey, message []byte) {
	if len(priv) != PrivateKeySize {
		panic(ErrPrivateKeySize)
	}
	var h [sha512.Size]byte
	var a, r, k, s scalar
	var R point
	hash(&h, priv[:SeedSize], nil, nil)
	secretScalar(&a, &h)
	hash(&h, h[32:], message, nil) // the prefix, then the message
	r.reduce(&h)
	scalarMultBase(&R, &r)
	R.bytes((*[32]byte)(sig[:32]))
	hash(&h, sig[:32], priv[SeedSize:], message)
	k.reduce(&h)
	s.mulAdd(&k, &a, &r)
	s.bytes((*[32]byte)(sig[32:]))
	clear(h[:])
	a, r, k, s, R = scalar{}, scalar{}, scalar{}, scalar{}, point{}
}

// Verify reports whether sig is a valid signature of message by pub (RFC
// 8032 section 5.1.7, without the cofactor, as crypto/ed25519). It rejects
// S of L and more. Unlike crypto/ed25519, it gives false instead of
// panicking when pub is not PublicKeySize bytes. It does not allocate.
func Verify(pub PublicKey, message, sig []byte) bool {
	if len(pub) != PublicKeySize || len(sig) != SignatureSize {
		return false
	}
	var s scalar
	if !s.setCanonical((*[32]byte)(sig[32:])) {
		return false
	}
	var A point
	if !A.setBytes((*[32]byte)(pub)) {
		return false
	}
	var h [sha512.Size]byte
	var k scalar
	hash(&h, sig[:32], pub, message)
	k.reduce(&h)
	A.neg()
	var R point
	doubleScalarMult(&R, &k, &A, &s) // S B - k A
	var enc [32]byte
	R.bytes(&enc)
	return enc == [32]byte(sig[:32])
}

// secretScalar sets a to the clamped first half of h (RFC 8032 section
// 5.1.5), reduced modulo L.
func secretScalar(a *scalar, h *[sha512.Size]byte) {
	var b [sha512.Size]byte
	copy(b[:32], h[:32])
	b[0] &= 248
	b[31] = b[31]&127 | 64
	a.reduce(&b)
	clear(b[:])
}

// hash sets out to the SHA-512 of a, b and c.
func hash(out *[sha512.Size]byte, a, b, c []byte) {
	var d sha512.Digest
	d.Reset(sha512.Init512, 0)
	d.Write(a)
	d.Write(b)
	d.Write(c)
	d.Sum(out)
}
