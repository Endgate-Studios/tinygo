// Package wpa derives WPA2-Personal keys (IEEE 802.11-2020 clause 12.7).
// All key material and scratch memory come from an injected allocator and are zeroed before they are freed.
//
// Not used over the air yet: espradio exports no raw 802.11, so the Espressif library does this work
// (espradio/DESIGN.md, Wi-Fi radio). It stays for a radio link that gives raw 802.11 frames.
package wpa

import (
	"crypto/sha1"
	"unsafe"

	"runtime/memory"
)

// Memory is where Keys allocates key material and scratch buffers.
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
	scratchLen       = sha1.BlockSize + 128
)

// Keys holds the PMK and PTK of one association.
type Keys struct {
	mem    Memory
	pmk    []byte
	ptk    []byte
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
	scratch, err := k.scratch()
	if err != nil {
		return err
	}
	pbkdf2SHA1(scratch, passphrase, ssid, pbkdf2Iterations, k.pmk)
	k.drop(scratch)
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
	scratch, err := k.scratch()
	if err != nil {
		return err
	}
	var data [2*6 + 2*32]byte
	sortedPair(data[0:12], aa[:], spa[:])
	sortedPair(data[12:], anonce[:], snonce[:])
	prf(scratch, k.pmk, "Pairwise key expansion", data[:], k.ptk)
	clear(data[:])
	k.drop(scratch)
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

// MIC computes the EAPOL-Key MIC for key descriptor version 2, HMAC-SHA1-128 (IEEE 802.11-2020 12.7.2).
// The MIC field of frame must be zero.
func (k *Keys) MIC(frame []byte, out *[MICLen]byte) error {
	if k.closed {
		return ErrClosed
	}
	if k.ptk == nil {
		return ErrNoPTK
	}
	scratch, err := k.scratchFor(len(frame))
	if err != nil {
		return err
	}
	var m mac
	m.init(k.KCK())
	sum := m.sum(scratch, frame, nil, nil, nil)
	copy(out[:], sum[:MICLen])
	m.wipe()
	clear(sum[:])
	k.drop(scratch)
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

func (k *Keys) scratch() ([]byte, error) {
	return k.scratchFor(0)
}

// scratchFor allocates one HMAC buffer for a message of up to msgLen bytes.
func (k *Keys) scratchFor(msgLen int) ([]byte, error) {
	n := scratchLen
	if need := sha1.BlockSize + msgLen; need > n {
		n = need
	}
	b := memory.MakeSlice[byte](k.mem, n)
	if b == nil {
		return nil, ErrNoMemory
	}
	return b, nil
}

func (k *Keys) drop(b []byte) {
	clear(b)
	k.mem.Free(unsafe.Pointer(unsafe.SliceData(b)))
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
