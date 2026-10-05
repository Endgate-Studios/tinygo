// Package wpa derives WPA2-Personal keys (IEEE 802.11-2020 clause 12.7).
// The key material comes from an injected allocator and is zeroed before it is freed. The
// HMAC states and buffers are on the stack and are cleared after use.
package wpa

import (
	"runtime/memory"
	"tinygo/crypto/aes"
)

// Memory is where Keys allocates key material.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilMemory   Error = "wpa: memory cannot be nil"
	ErrNoMemory    Error = "wpa: out of memory"
	ErrPassphrase  Error = "wpa: passphrase must be 8 to 63 characters"
	ErrSSID        Error = "wpa: SSID must be 1 to 32 bytes"
	ErrPMKLen      Error = "wpa: PMK must be 32 bytes"
	ErrNoPMK       Error = "wpa: no PMK, set the passphrase first"
	ErrNoPTK       Error = "wpa: no PTK, derive it first"
	ErrClosed      Error = "wpa: keys are closed"
	ErrWrappedSize Error = "wpa: wrapped key must be a multiple of 8 bytes and at least 24"
	ErrUnwrap      Error = "wpa: key unwrap integrity check failed"
)

// Sizes in bytes (IEEE 802.11-2020 table 12-8 for CCMP-128).
const (
	PMKLen = 32
	KCKLen = 16
	KEKLen = 16
	TKLen  = 16
	PTKLen = KCKLen + KEKLen + TKLen
	MICLen = 16

	pbkdf2Iterations = 4096
)

// Keys holds the PMK and PTK of one association.
type Keys struct {
	mem    Memory
	pmk    []byte
	ptk    []byte
	sae    bool
	closed bool
}

// New returns empty Keys that allocate from mem.
func New(mem Memory) (*Keys, error) {
	if mem == nil {
		return nil, ErrNilMemory
	}
	return &Keys{mem: mem}, nil
}

// Close zeroes and frees all key material. Keys cannot be used after Close.
func (k *Keys) Close() {
	k.release(&k.pmk)
	k.release(&k.ptk)
	k.closed = true
}

// UseSAE selects the key derivation and the MIC of the SAE AKM (IEEE 802.11-2020 table 12-11):
// KDF-SHA256 for the PTK and AES-128-CMAC with the KCK for the EAPOL-Key MIC. Call it before
// DerivePTK.
func (k *Keys) UseSAE() { k.sae = true }

// SetPassphrase derives the PMK from a WPA passphrase and SSID (IEEE 802.11-2020 J.4).
func (k *Keys) SetPassphrase(passphrase, ssid []byte) error {
	if k.closed {
		return ErrClosed
	}
	if len(passphrase) < 8 || len(passphrase) > 63 {
		return ErrPassphrase
	}
	if len(ssid) < 1 || len(ssid) > 32 {
		return ErrSSID
	}
	if err := k.hold(&k.pmk, PMKLen); err != nil {
		return err
	}
	pbkdf2SHA1(passphrase, ssid, pbkdf2Iterations, k.pmk)
	return nil
}

// SetPMK sets the PMK directly. WPA2-Personal uses the 32-byte pre-shared key as the
// PMK (IEEE 802.11-2020 12.7.1.3).
func (k *Keys) SetPMK(pmk []byte) error {
	if k.closed {
		return ErrClosed
	}
	if len(pmk) != PMKLen {
		return ErrPMKLen
	}
	if err := k.hold(&k.pmk, PMKLen); err != nil {
		return err
	}
	copy(k.pmk, pmk)
	return nil
}

// DerivePTK derives the PTK from the PMK, both MAC addresses and both nonces (IEEE 802.11-2020 12.7.1.3).
// The order of the arguments does not matter, because the inputs are sorted.
func (k *Keys) DerivePTK(aa, spa [6]byte, anonce, snonce [32]byte) error {
	if k.closed {
		return ErrClosed
	}
	if k.pmk == nil {
		return ErrNoPMK
	}
	if err := k.hold(&k.ptk, PTKLen); err != nil {
		return err
	}
	var data [2*6 + 2*32]byte
	sortedPair(data[0:12], aa[:], spa[:])
	sortedPair(data[12:], anonce[:], snonce[:])
	if k.sae {
		KDFSHA256(k.pmk, "Pairwise key expansion", data[:], k.ptk)
	} else {
		prf(k.pmk, "Pairwise key expansion", data[:], k.ptk)
	}
	clear(data[:])
	return nil
}

// PMK returns the pairwise master key, or nil before SetPassphrase or SetPMK.
// The slice is valid until Close.
func (k *Keys) PMK() []byte { return k.pmk }

// KCK returns the key confirmation key. The slice is valid until Close.
func (k *Keys) KCK() []byte { return k.part(0, KCKLen) }

// KEK returns the key encryption key. The slice is valid until Close.
func (k *Keys) KEK() []byte { return k.part(KCKLen, KEKLen) }

// TK returns the temporal key for CCMP. The slice is valid until Close.
func (k *Keys) TK() []byte { return k.part(KCKLen+KEKLen, TKLen) }

// MIC computes the EAPOL-Key MIC (IEEE 802.11-2020 12.7.2): HMAC-SHA1-128 for key descriptor
// version 2, or AES-128-CMAC after UseSAE. The MIC field of frame must be zero.
func (k *Keys) MIC(frame []byte, out *[MICLen]byte) error {
	if k.closed {
		return ErrClosed
	}
	if k.ptk == nil {
		return ErrNoPTK
	}
	if k.sae {
		var c aes.Cipher
		if err := c.Init(k.mem, k.KCK()); err != nil {
			return ErrNoMemory
		}
		err := c.CMAC(out, frame)
		c.Close()
		return err
	}
	var m mac
	m.init(k.KCK())
	sum := m.sum(frame, nil, nil, nil)
	copy(out[:], sum[:MICLen])
	m.wipe()
	clear(sum[:])
	return nil
}

func (k *Keys) part(off, n int) []byte {
	if k.ptk == nil {
		return nil
	}
	return k.ptk[off : off+n]
}

// hold makes *buf a block of n bytes from mem, and reuses it when it exists.
func (k *Keys) hold(buf *[]byte, n int) error {
	if *buf != nil {
		return nil
	}
	b := memory.MakeSlice[byte](k.mem, n)
	if b == nil {
		return ErrNoMemory
	}
	*buf = b
	return nil
}

func (k *Keys) release(buf *[]byte) {
	if *buf == nil {
		return
	}
	clear(*buf)
	memory.FreeSlice(k.mem, *buf)
	*buf = nil
}

// sortedPair writes the smaller of a and b, then the larger, into dst.
// It picks the order first and does not swap the slices, because TinyGo then moves the arrays to the heap.
func sortedPair(dst, a, b []byte) {
	if greater(a, b) {
		copy(dst, b)
		copy(dst[len(b):], a)
		return
	}
	copy(dst, a)
	copy(dst[len(a):], b)
}

func greater(a, b []byte) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
