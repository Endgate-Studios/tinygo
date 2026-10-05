// Package p256 is the NIST P-256 curve: ECDH for LE Secure Connections pairing (Core spec v5.4 Vol 3
// Part H 2.3.5.6) and the operations of SAE group 19 for WPA3 (IEEE 802.11-2020 12.4).
// Its working points come from an injected allocator that can free, and it has no precomputed tables.
package p256

import (
	"io"
	"unsafe"

	"runtime/memory"
)

// Memory is where the working points of one operation come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilMemory Error = "p256: memory cannot be nil"
	ErrNoMemory  Error = "p256: out of memory"
	ErrScalar    Error = "p256: private key must be in 1 to n-1"
	ErrPoint     Error = "p256: public key is not on the curve"
	ErrRandom    Error = "p256: random source failed"
)

// n is the group order (FIPS 186-4 appendix D.1.2.3), big endian.
var n = [32]byte{
	0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xbc, 0xe6, 0xfa, 0xad, 0xa7, 0x17, 0x9e, 0x84, 0xf3, 0xb9, 0xca, 0xc2, 0xfc, 0x63, 0x25, 0x51,
}

// work is the working memory of one operation.
type work struct {
	acc, t, base point
	table        [16]point
	x, y         element
}

// PublicKey computes the public key of priv. All values are big endian.
func PublicKey(mem Memory, priv, x, y *[32]byte) error {
	return mul(mem, priv, nil, nil, x, y)
}

// SharedSecret computes the x coordinate of priv times the peer public key, the DHKey of LE Secure
// Connections. It rejects a peer key that is not on the curve.
func SharedSecret(mem Memory, priv, peerX, peerY, out *[32]byte) error {
	return mul(mem, priv, peerX, peerY, out, nil)
}

// GenerateKey draws a private key from rng and computes its public key.
func GenerateKey(mem Memory, rng io.Reader, priv, x, y *[32]byte) error {
	for tries := 0; tries < 64; tries++ {
		if _, err := io.ReadFull(rng, priv[:]); err != nil {
			clear(priv[:])
			return ErrRandom
		}
		if validScalar(priv) {
			return PublicKey(mem, priv, x, y)
		}
	}
	clear(priv[:])
	return ErrRandom
}

// OnCurve reports whether (x, y) is a point on the curve, with both coordinates below p.
func OnCurve(x, y *[32]byte) bool {
	var ex, ey element
	return feFromBytes(&ex, x) && feFromBytes(&ey, y) && onCurve(&ex, &ey)
}

// mul computes priv times the peer point, or times G when peerX is nil, and writes the affine result.
func mul(mem Memory, priv, peerX, peerY, outX, outY *[32]byte) error {
	if mem == nil {
		return ErrNilMemory
	}
	if !validScalar(priv) {
		return ErrScalar
	}
	w := memory.New[work](mem)
	if w == nil {
		return ErrNoMemory
	}
	defer release(mem, w)
	if peerX == nil {
		w.base = gen
	} else {
		if !feFromBytes(&w.base.x, peerX) || !feFromBytes(&w.base.y, peerY) || !onCurve(&w.base.x, &w.base.y) {
			return ErrPoint
		}
		w.base.z = element{1}
	}
	scalarMult(&w.acc, &w.t, &w.table, &w.base, priv)
	if !w.acc.affine(&w.x, &w.y) {
		return ErrPoint // unreachable for a valid scalar and point on this prime order curve
	}
	feToBytes(outX, &w.x)
	if outY != nil {
		feToBytes(outY, &w.y)
	}
	return nil
}

// validScalar reports in constant time whether 0 < k < n.
func validScalar(k *[32]byte) bool {
	var nonzero byte
	var borrow int
	for i := 31; i >= 0; i-- {
		nonzero |= k[i]
		d := int(k[i]) - int(n[i]) - borrow
		borrow = (d >> 8) & 1
	}
	return nonzero != 0 && borrow == 1
}

func release(mem Memory, w *work) {
	*w = work{}
	mem.Free(unsafe.Pointer(w))
}
