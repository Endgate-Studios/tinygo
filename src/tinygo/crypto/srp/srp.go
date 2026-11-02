// Package srp is SRP-6a (RFC 5054) with the 3072-bit group of RFC 5054
// appendix A (g = 5) and SHA-512, as HomeKit and AirPlay 2 pair-setup use it.
// The Server is the accessory, the Client the controller. The username is
// "Pair-Setup"; AirPlay 2 transient pairing uses the password "3939". The
// working numbers live in the Server and Client, so an exchange does not
// call the GC heap, and tinygo/crypto/internal/mpi multiplies, so the RSA
// accelerator does it on the ESP32-S3.
//
// The conventions are those of Apple's HomeKit ADK
// (PAL/Crypto/OpenSSL/HAPOpenSSL.c), which interoperates with Apple devices.
// H is SHA-512, PAD pads with zero bytes to 384, the length of N, STRIP
// removes leading zero bytes, and the salt s is always its 16 bytes:
//
//	x  = H(s | H(I | ":" | P))
//	v  = g^x
//	k  = H(N | PAD(g))
//	A  = g^a, B = k v + g^b, both sent as PAD
//	u  = H(PAD(A) | PAD(B))
//	S  = (A v^u)^b = (B - k g^x)^(a + u x)
//	K  = H(STRIP(S))
//	M1 = H(H(N) xor H(g) | H(I) | s | STRIP(A) | STRIP(B) | K), H(g) over one byte
//	M2 = H(PAD(A) | M1 | K)
//
// shairport-sync (pair_ap/pair_homekit.c) and HAP-python (pyhap/hsrp.py)
// agree on k, u, K and M1 when no value starts with a zero byte. They differ
// from the ADK and from each other in the rare cases where one does:
// shairport-sync strips the salt in M1 and A in M2, HAP-python hashes A as
// received in M1. This package follows the ADK.
//
// Mul and the exponentiation take a time that does not depend on the values.
// The proofs are compared in constant time. STRIP(S) leaks the number of
// leading zero bytes of S through the hash length, as in the ADK.
package srp

import (
	"crypto/subtle"

	"tinygo/crypto/internal/mpi"
	"tinygo/crypto/sha512"
)

const (
	// SaltSize is the length of the salt s.
	SaltSize = 16
	// SecretSize is the length of the secret exponents a and b.
	SecretSize = 32
	// PublicSize is the length of N, and of A, B and the verifier v.
	PublicSize = 384
	// KeySize is the length of the session key K.
	KeySize = sha512.Size
	// ProofSize is the length of the proofs M1 and M2.
	ProofSize = sha512.Size
	// Username is the username I of HomeKit and AirPlay 2 pair-setup.
	Username = "Pair-Setup"
)

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

// Error returns the message.
func (e Error) Error() string { return string(e) }

const (
	// ErrPublicKey tells that A or B is empty, longer than N or 0 mod N.
	ErrPublicKey Error = "srp: public key must be 1 to 384 bytes and not 0 mod N"
	// ErrVerifier tells that the verifier is 0 or not below N.
	ErrVerifier Error = "srp: verifier must be in 1 to N-1"
	// ErrSecret tells that the secret exponent a or b is 0.
	ErrSecret Error = "srp: secret exponent cannot be 0"
	// ErrProof tells that a proof does not match, for example after a wrong
	// password.
	ErrProof Error = "srp: proof mismatch"
	// ErrState tells that Init was not called or the exchange is over.
	ErrState Error = "srp: exchange not started or already finished"
)

const (
	words       = PublicSize / 4
	secretWords = SecretSize / 4
	hashWords   = sha512.Size / 4
	g           = 5
)

// number is a value below N, 32-bit words with the least significant first.
type number [words]uint32

// n is the prime of RFC 5054 appendix A (RFC 3526 section 4).
var n = number{
	0xffffffff, 0xffffffff, 0xa93ad2ca, 0x4b82d120, 0xe0fd108e, 0x43db5bfc,
	0x74e5ab31, 0x08e24fa0, 0xbad946e2, 0x770988c0, 0x7a615d6c, 0xbbe11757,
	0x177b200c, 0x521f2b18, 0x3ec86a64, 0xd8760273, 0xd98a0864, 0xf12ffa06,
	0x1ad2ee6b, 0xcee3d226, 0x4a25619d, 0x1e8c94e0, 0xdb0933d7, 0xabf5ae8c,
	0xa6e1e4c7, 0xb3970f85, 0x5d060c7d, 0x8aea7157, 0x58dbef0a, 0xecfb8504,
	0xdf1cba64, 0xa85521ab, 0x04507a33, 0xad33170d, 0x8aaac42d, 0x15728e5a,
	0x98fa0510, 0x15d22618, 0xea956ae5, 0x3995497c, 0x95581718, 0xde2bcbf6,
	0x6f4c52c9, 0xb5c55df0, 0xec07a28f, 0x9b2783a2, 0x180e8603, 0xe39e772c,
	0x2e36ce3b, 0x32905e46, 0xca18217c, 0xf1746c08, 0x4abc9804, 0x670c354e,
	0x7096966d, 0x9ed52907, 0x208552bb, 0x1c62f356, 0xdca3ad96, 0x83655d23,
	0xfd24cf5f, 0x69163fa8, 0x1c55d39a, 0x98da4836, 0xa163bf05, 0xc2007cb8,
	0xece45b3d, 0x49286651, 0x7c4b1fe6, 0xae9f2411, 0x5a899fa5, 0xee386bfb,
	0xf406b7ed, 0x0bff5cb6, 0xa637ed6b, 0xf44c42e9, 0x625e7ec6, 0xe485b576,
	0x6d51c245, 0x4fe1356d, 0xf25f1437, 0x302b0a6d, 0xcd3a431b, 0xef9519b3,
	0x8e3404dd, 0x514a0879, 0x3b139b22, 0x020bbea6, 0x8a67cc74, 0x29024e08,
	0x80dc1cd1, 0xc4c6628b, 0x2168c234, 0xc90fdaa2, 0xffffffff, 0xffffffff,
}

var (
	modN mpi.Modulus
	rrN  number
	hk   [sha512.Size]byte // k = H(N | PAD(g))
	hNg  [sha512.Size]byte // H(N) xor H(g)
)

func init() {
	modN.Init(n[:], rrN[:])
	gb := [1]byte{g}
	d := digest()
	writeNumber(&d, &n, false)
	writeZeros(&d, PublicSize-1)
	d.Write(gb[:])
	d.Sum(&hk)
	d = digest()
	writeNumber(&d, &n, false)
	d.Sum(&hNg)
	var hg [sha512.Size]byte
	d = digest()
	d.Write(gb[:])
	d.Sum(&hg)
	for i := range hNg {
		hNg[i] ^= hg[i]
	}
}

// Verifier writes the verifier v = g^x of username and password with salt
// to v. Its 1.2 KB of working numbers are locals, which TinyGo puts on the
// GC heap when the stack is small; Server.InitPassword uses the Server
// instead.
func Verifier(v *[PublicSize]byte, username, password []byte, salt *[SaltSize]byte) {
	var w [3]number
	verifier(&w[0], username, password, salt, &w[1], &w[2])
	toBytes(v, &w[0])
	clear(w[:])
}

// verifier sets v = g^x, with acc and t as working numbers.
func verifier(v *number, username, password []byte, salt *[SaltSize]byte, acc, t *number) {
	var x [hashWords]uint32
	privateKey(&x, username, password, salt)
	generator(v)
	exp(v, v, x[:], true, acc, t)
	clear(x[:])
}

// Server is the accessory side of one exchange. Use it as a value after
// Init. It holds its working numbers, about 2.4 KB in all, so put it in a
// global or another long-lived struct, not on a small stack. Verify ends
// the exchange and clears the Server.
type Server struct {
	v    number
	w    [4]number // working numbers
	b    [secretWords]uint32
	pub  [PublicSize]byte // B
	salt [SaltSize]byte
	hI   [sha512.Size]byte // H(I)
	ok   bool
}

// Init starts an exchange for username with verifier v and salt, and the
// secret exponent b, 32 random bytes. It computes B = k v + g^b.
func (s *Server) Init(username []byte, v *[PublicSize]byte, salt *[SaltSize]byte, b *[SecretSize]byte) error {
	s.clear()
	fromBytes(&s.v, v[:])
	if isZero(&s.v) || !belowN(&s.v) {
		s.clear()
		return ErrVerifier
	}
	return s.start(username, salt, b)
}

// InitPassword is Init with the verifier of username and password, which
// it computes.
func (s *Server) InitPassword(username, password []byte, salt *[SaltSize]byte, b *[SecretSize]byte) error {
	s.clear()
	verifier(&s.v, username, password, salt, &s.w[0], &s.w[1])
	return s.start(username, salt, b)
}

// start computes B = k v + g^b for the verifier in s.v.
func (s *Server) start(username []byte, salt *[SaltSize]byte, b *[SecretSize]byte) error {
	if !secretFromBytes(&s.b, b) {
		s.clear()
		return ErrSecret
	}
	w := &s.w
	fromBytes(&w[0], hk[:])
	modN.Mul(w[0][:], w[0][:], s.v[:])
	generator(&w[1])
	exp(&w[1], &w[1], s.b[:], true, &w[2], &w[3])
	modN.Add(w[0][:], w[0][:], w[1][:])
	toBytes(&s.pub, &w[0])
	clear(w[:])
	s.salt = *salt
	d := digest()
	d.Write(username)
	d.Sum(&s.hI)
	s.ok = true
	return nil
}

// PublicKey returns B, to send with the salt.
func (s *Server) PublicKey() *[PublicSize]byte { return &s.pub }

// Salt returns the salt, to send with B.
func (s *Server) Salt() *[SaltSize]byte { return &s.salt }

// Verify checks the client's public key a and proof m1. On success it
// writes the server proof M2 to m2 and the session key K to key. It fails
// with ErrPublicKey when a is 0 mod N and with ErrProof when m1 is wrong,
// for example after a wrong password. Either way the exchange is over.
func (s *Server) Verify(a []byte, m1, m2 *[ProofSize]byte, key *[KeySize]byte) error {
	err := s.verify(a, m1, m2, key)
	s.clear()
	return err
}

func (s *Server) verify(a []byte, m1, m2 *[ProofSize]byte, key *[KeySize]byte) error {
	if !s.ok {
		return ErrState
	}
	if !s.premaster(a) {
		return ErrPublicKey
	}
	var k, want [sha512.Size]byte
	sessionKey(&k, &s.w[1])
	proof(&want, &s.hI, &s.salt, a, s.pub[:], &k)
	if subtle.ConstantTimeCompare(want[:], m1[:]) != 1 {
		clear(k[:])
		return ErrProof
	}
	serverProof(m2, a, m1, &k)
	*key = k
	clear(k[:])
	return nil
}

// premaster sets s.w[1] = S = (A v^u)^b. It reports false when a is not 1
// to PublicSize bytes or is 0 mod N.
func (s *Server) premaster(a []byte) bool {
	w := &s.w
	if !publicFromBytes(&w[0], a) {
		return false
	}
	var u [hashWords]uint32
	scramble(&u, a, s.pub[:])
	exp(&w[1], &s.v, u[:], false, &w[2], &w[3])
	modN.Mul(w[1][:], w[0][:], w[1][:])
	exp(&w[1], &w[1], s.b[:], true, &w[2], &w[3])
	return true
}

func (s *Server) clear() {
	clear(s.v[:])
	clear(s.w[:])
	clear(s.b[:])
	clear(s.pub[:])
	clear(s.salt[:])
	clear(s.hI[:])
	s.ok = false
}

// Client is the controller side of one exchange. Use it as a value after
// Init. It holds its working numbers, about 2.4 KB in all, so put it in a
// global or another long-lived struct, not on a small stack.
type Client struct {
	w     [5]number // working numbers
	a     [secretWords]uint32
	pub   [PublicSize]byte // A
	m2    [ProofSize]byte  // the expected M2
	state uint8            // 1 after Init, 2 after Proof
}

// Init starts an exchange with the secret exponent a, 32 random bytes. It
// computes A = g^a.
func (c *Client) Init(a *[SecretSize]byte) error {
	c.clear()
	if !secretFromBytes(&c.a, a) {
		c.clear()
		return ErrSecret
	}
	w := &c.w
	generator(&w[0])
	exp(&w[0], &w[0], c.a[:], true, &w[1], &w[2])
	toBytes(&c.pub, &w[0])
	clear(w[:])
	c.state = 1
	return nil
}

// PublicKey returns A, to send with M1.
func (c *Client) PublicKey() *[PublicSize]byte { return &c.pub }

// Proof takes the server's salt and public key b, and writes the client
// proof M1 to m1 and the session key K to key. It fails with ErrPublicKey
// when b is not 1 to PublicSize bytes, b is 0 mod N or u is 0.
func (c *Client) Proof(username, password []byte, salt *[SaltSize]byte, b []byte, m1 *[ProofSize]byte, key *[KeySize]byte) error {
	if c.state != 1 {
		return ErrState
	}
	w := &c.w
	if !publicFromBytes(&w[0], b) {
		clear(w[0][:])
		return ErrPublicKey
	}
	var u, x [hashWords]uint32
	scramble(&u, c.pub[:], b)
	if isZeroWords(u[:]) {
		clear(w[0][:])
		return ErrPublicKey
	}
	privateKey(&x, username, password, salt)
	// base = B - k g^x, then S = base^a (base^u)^x.
	generator(&w[1])
	exp(&w[1], &w[1], x[:], true, &w[3], &w[4])
	fromBytes(&w[2], hk[:])
	modN.Mul(w[1][:], w[2][:], w[1][:])
	modN.Sub(w[2][:], w[0][:], w[1][:])
	exp(&w[0], &w[2], c.a[:], true, &w[3], &w[4])
	exp(&w[1], &w[2], u[:], false, &w[3], &w[4])
	exp(&w[1], &w[1], x[:], true, &w[3], &w[4])
	modN.Mul(w[0][:], w[0][:], w[1][:])
	clear(x[:])
	var k, hI [sha512.Size]byte
	sessionKey(&k, &w[0])
	clear(w[:])
	d := digest()
	d.Write(username)
	d.Sum(&hI)
	proof(m1, &hI, salt, c.pub[:], b, &k)
	serverProof(&c.m2, c.pub[:], m1, &k)
	*key = k
	clear(k[:])
	clear(c.a[:])
	c.state = 2
	return nil
}

// Verify checks the server proof m2. The exchange is over after it.
func (c *Client) Verify(m2 *[ProofSize]byte) error {
	state := c.state
	ok := subtle.ConstantTimeCompare(c.m2[:], m2[:]) == 1
	c.clear()
	if state != 2 {
		return ErrState
	}
	if !ok {
		return ErrProof
	}
	return nil
}

func (c *Client) clear() {
	clear(c.w[:])
	clear(c.a[:])
	clear(c.pub[:])
	clear(c.m2[:])
	c.state = 0
}

func digest() sha512.Digest {
	var d sha512.Digest
	d.Reset(sha512.Init512, 0)
	return d
}

// privateKey sets x = H(s | H(I | ":" | P)).
func privateKey(x *[hashWords]uint32, username, password []byte, salt *[SaltSize]byte) {
	var h [sha512.Size]byte
	d := digest()
	d.Write(username)
	colon := [1]byte{':'}
	d.Write(colon[:])
	d.Write(password)
	d.Sum(&h)
	d = digest()
	d.Write(salt[:])
	d.Write(h[:])
	d.Sum(&h)
	hashToWords(x, &h)
	clear(h[:])
}

// scramble sets u = H(PAD(A) | PAD(B)), for a and b of at most PublicSize
// bytes.
func scramble(u *[hashWords]uint32, a, b []byte) {
	var h [sha512.Size]byte
	d := digest()
	writePadded(&d, a)
	writePadded(&d, b)
	d.Sum(&h)
	hashToWords(u, &h)
}

// sessionKey writes K = H(STRIP(S)) to k.
func sessionKey(k *[sha512.Size]byte, s *number) {
	d := digest()
	writeNumber(&d, s, true)
	d.Sum(k)
}

// proof writes M1 = H(H(N) xor H(g) | H(I) | s | STRIP(A) | STRIP(B) | K) to m1.
func proof(m1, hI *[sha512.Size]byte, salt *[SaltSize]byte, a, b []byte, k *[sha512.Size]byte) {
	d := digest()
	d.Write(hNg[:])
	d.Write(hI[:])
	d.Write(salt[:])
	d.Write(strip(a))
	d.Write(strip(b))
	d.Write(k[:])
	d.Sum(m1)
}

// serverProof writes M2 = H(PAD(A) | M1 | K) to m2.
func serverProof(m2 *[ProofSize]byte, a []byte, m1 *[ProofSize]byte, k *[sha512.Size]byte) {
	d := digest()
	writePadded(&d, a)
	d.Write(m1[:])
	d.Write(k[:])
	d.Sum(m2)
}

func strip(b []byte) []byte {
	for len(b) > 0 && b[0] == 0 {
		b = b[1:]
	}
	return b
}

// writeZeros hashes n zero bytes.
func writeZeros(d *sha512.Digest, n int) {
	var zero [64]byte
	for n > 0 {
		k := min(n, len(zero))
		d.Write(zero[:k])
		n -= k
	}
}

// writePadded hashes PAD(b), for b of at most PublicSize bytes.
func writePadded(d *sha512.Digest, b []byte) {
	writeZeros(d, PublicSize-len(b))
	d.Write(b)
}

// writeNumber hashes z as PublicSize big-endian bytes, without the leading
// zero bytes with strip.
func writeNumber(d *sha512.Digest, z *number, strip bool) {
	var buf [64]byte
	k := 0
	for i := PublicSize - 1; i >= 0; i-- {
		c := byte(z[i/4] >> (8 * (i % 4)))
		if strip && c == 0 {
			continue
		}
		strip = false
		buf[k] = c
		k++
		if k == len(buf) {
			d.Write(buf[:])
			k = 0
		}
	}
	d.Write(buf[:k])
	clear(buf[:])
}

// hashToWords reads a digest into words, the least significant first.
func hashToWords(z *[hashWords]uint32, h *[sha512.Size]byte) {
	for i := range z {
		j := sha512.Size - 4 - 4*i
		z[i] = uint32(h[j])<<24 | uint32(h[j+1])<<16 | uint32(h[j+2])<<8 | uint32(h[j+3])
	}
}

// publicFromBytes reads a public key of 1 to PublicSize bytes into z,
// reduced mod N. It reports false when the length is wrong or the key is 0
// mod N. The key is below 2^3072 < 2N, so one subtraction reduces it.
func publicFromBytes(z *number, b []byte) bool {
	if len(b) == 0 || len(b) > PublicSize {
		return false
	}
	fromBytes(z, b)
	if !belowN(z) {
		var borrow uint64
		for i := range z {
			v := uint64(z[i]) - uint64(n[i]) - borrow
			z[i], borrow = uint32(v), (v>>32)&1
		}
	}
	return !isZero(z)
}

// secretFromBytes reads a secret exponent and reports whether it is not 0.
func secretFromBytes(z *[secretWords]uint32, b *[SecretSize]byte) bool {
	clear(z[:])
	for i := 0; i < SecretSize; i++ {
		z[i/4] |= uint32(b[SecretSize-1-i]) << (8 * (i % 4))
	}
	return !isZeroWords(z[:])
}

// fromBytes reads big-endian bytes, at most PublicSize, into z.
func fromBytes(z *number, b []byte) {
	*z = number{}
	for i := 0; i < len(b); i++ {
		z[i/4] |= uint32(b[len(b)-1-i]) << (8 * (i % 4))
	}
}

// toBytes writes z as PublicSize big-endian bytes.
func toBytes(b *[PublicSize]byte, z *number) {
	for i := 0; i < PublicSize; i++ {
		b[PublicSize-1-i] = byte(z[i/4] >> (8 * (i % 4)))
	}
}

func generator(z *number) {
	*z = number{}
	z[0] = g
}

// isZero reports whether z is 0, in a time that does not depend on z.
func isZero(z *number) bool { return isZeroWords(z[:]) }

func isZeroWords(z []uint32) bool {
	var acc uint32
	for _, w := range z {
		acc |= w
	}
	return acc == 0
}

// belowN reports whether z < N, in a time that does not depend on z.
func belowN(z *number) bool {
	var borrow uint64
	for i := range z {
		v := uint64(z[i]) - uint64(n[i]) - borrow
		borrow = (v >> 32) & 1
	}
	return borrow == 1
}

// exp sets z = x^e mod N, for e of 32-bit words, the least significant
// first, by square and multiply over all bits of e, with acc and t as
// working numbers. With secret, it multiplies at every bit and keeps the
// product with a mask, so the time does not depend on e. z can be x.
func exp(z, x *number, e []uint32, secret bool, acc, t *number) {
	*acc = number{}
	acc[0] = 1
	for i := 32*len(e) - 1; i >= 0; i-- {
		modN.Mul(acc[:], acc[:], acc[:])
		bit := e[i/32] >> (i % 32) & 1
		if !secret {
			if bit == 1 {
				modN.Mul(acc[:], acc[:], x[:])
			}
			continue
		}
		modN.Mul(t[:], acc[:], x[:])
		mask := -bit
		for j := range acc {
			acc[j] = t[j]&mask | acc[j]&^mask
		}
	}
	*z = *acc
	clear(acc[:])
	clear(t[:])
}
