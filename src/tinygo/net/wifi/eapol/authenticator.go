package eapol

import (
	"crypto/subtle"
	"encoding/binary"
	"io"

	"runtime/memory"
	"tinygo/net/wifi/ieee80211"
	"tinygo/net/wifi/wpa"
)

// The access point side of WPA2-Personal with CCMP: the 4-way handshake of
// IEEE 802.11-2020 12.7.6 for each station, and one GTK for all stations.

const (
	// MaxResends is the number of times Resend sends message 1 or 3 again,
	// as dot11RSNAConfigPairwiseUpdateCount of hostapd.
	MaxResends = 3

	gtkLen   = 16
	gtkKeyID = 1

	ErrStationRSN Error = "eapol: RSN element of the station is not WPA2-PSK with CCMP"
	ErrTimeout    Error = "eapol: station did not answer the 4-way handshake"
)

// Authenticator holds the PMK, the RSN element and the GTK of one access
// point.
type Authenticator struct {
	mem    Memory
	rng    io.Reader
	pmk    [wpa.PMKLen]byte
	aa     ieee80211.MAC
	rsn    []byte
	gtk    [gtkLen]byte
	closed bool
}

// NewAuthenticator derives the PMK for passphrase and ssid and makes a random
// GTK. aa is the MAC address of the access point and rsn its full RSN element.
func NewAuthenticator(mem Memory, rng io.Reader, aa ieee80211.MAC, rsn, passphrase, ssid []byte) (*Authenticator, error) {
	if mem == nil {
		return nil, ErrNilMemory
	}
	if rng == nil {
		return nil, ErrNilRandom
	}
	keys, err := wpa.New(mem)
	if err != nil {
		return nil, err
	}
	defer keys.Close()
	if err := keys.SetPassphrase(passphrase, ssid); err != nil {
		return nil, err
	}
	return NewAuthenticatorPMK(mem, rng, aa, rsn, keys.PMK())
}

// NewAuthenticatorPMK is NewAuthenticator with a PMK that is already derived.
func NewAuthenticatorPMK(mem Memory, rng io.Reader, aa ieee80211.MAC, rsn, pmk []byte) (*Authenticator, error) {
	if mem == nil {
		return nil, ErrNilMemory
	}
	if rng == nil {
		return nil, ErrNilRandom
	}
	if len(pmk) != wpa.PMKLen {
		return nil, wpa.ErrPMKLen
	}
	a := &Authenticator{mem: mem, rng: rng, aa: aa}
	copy(a.pmk[:], pmk)
	if a.rsn = memory.MakeSlice[byte](mem, len(rsn)); a.rsn == nil {
		return nil, ErrNoMemory
	}
	copy(a.rsn, rsn)
	if _, err := io.ReadFull(rng, a.gtk[:]); err != nil {
		a.Close()
		return nil, ErrRandom
	}
	return a, nil
}

// GTK returns the group key and its key ID, for the transmit key of the
// access point.
func (a *Authenticator) GTK() ([]byte, uint8) { return a.gtk[:], gtkKeyID }

// Close zeroes the keys and frees the memory.
func (a *Authenticator) Close() {
	clear(a.pmk[:])
	clear(a.gtk[:])
	if a.rsn != nil {
		clear(a.rsn)
		memory.FreeSlice(a.mem, a.rsn)
		a.rsn = nil
	}
	a.closed = true
}

type hsStage uint8

const (
	hsIdle hsStage = iota
	hsSentM1
	hsSentM3
	hsDone
)

// Handshake is the 4-way handshake with one station.
type Handshake struct {
	a      *Authenticator
	keys   *wpa.Keys
	spa    ieee80211.MAC
	staRSN []byte
	anonce [nonceLen]byte
	snonce [nonceLen]byte
	replay uint64
	stage  hsStage
	sends  int
}

// NewHandshake starts the state of station spa. staRSN is the full RSN
// element of its association request. It must ask for PSK and CCMP.
func (a *Authenticator) NewHandshake(spa ieee80211.MAC, staRSN []byte) (*Handshake, error) {
	if a.closed {
		return nil, ErrClosed
	}
	if !wpa2PSKCCMP(staRSN) {
		return nil, ErrStationRSN
	}
	keys, err := wpa.New(a.mem)
	if err != nil {
		return nil, err
	}
	if err := keys.SetPMK(a.pmk[:]); err != nil {
		keys.Close()
		return nil, err
	}
	h := &Handshake{a: a, keys: keys, spa: spa}
	if h.staRSN = memory.MakeSlice[byte](a.mem, len(staRSN)); h.staRSN == nil {
		keys.Close()
		return nil, ErrNoMemory
	}
	copy(h.staRSN, staRSN)
	return h, nil
}

// Start writes message 1 into reply and returns its length.
func (h *Handshake) Start(reply []byte) (int, error) {
	if _, err := io.ReadFull(h.a.rng, h.anonce[:]); err != nil {
		return 0, ErrRandom
	}
	h.stage, h.sends = hsSentM1, 0
	return h.message1(reply)
}

// Resend writes the last message again with a new replay counter. It returns
// ErrTimeout after MaxResends, and then the access point removes the station.
func (h *Handshake) Resend(reply []byte) (int, error) {
	if h.sends >= MaxResends {
		return 0, ErrTimeout
	}
	h.sends++
	switch h.stage {
	case hsSentM1:
		return h.message1(reply)
	case hsSentM3:
		return h.message3(reply)
	}
	return 0, ErrState
}

// Waiting reports whether the handshake waits for an answer of the station.
func (h *Handshake) Waiting() bool { return h.stage == hsSentM1 || h.stage == hsSentM3 }

// Handle processes one EAPOL frame of the station, from the EAPOL header.
// When n > 0, send reply[:n] to the station. done is true after message 4.
func (h *Handshake) Handle(frame, reply []byte) (n int, done bool, err error) {
	f, err := parseKeyFrame(frame, infoVersionAES)
	if err != nil {
		return 0, false, err
	}
	if f.info&(infoPairwise|infoMIC) != infoPairwise|infoMIC || f.info&(infoAck|infoRequest|infoError) != 0 {
		return 0, false, ErrState
	}
	if f.replay != h.replay {
		return 0, false, ErrReplay
	}
	switch {
	case h.stage == hsSentM1 && len(f.data) > 0:
		copy(h.snonce[:], f.nonce)
		if err := h.keys.DerivePTK(h.a.aa, h.spa, h.anonce, h.snonce); err != nil {
			return 0, false, err
		}
		if err := h.verify(&f); err != nil {
			return 0, false, err
		}
		if got := findRSN(f.data); got == nil || subtle.ConstantTimeCompare(got, h.staRSN) != 1 {
			return 0, false, ErrRSNMismatch
		}
		h.stage, h.sends = hsSentM3, 0
		n, err := h.message3(reply)
		return n, false, err
	case h.stage == hsSentM3 && len(f.data) == 0:
		if err := h.verify(&f); err != nil {
			return 0, false, err
		}
		h.stage = hsDone
		return 0, true, nil
	}
	return 0, false, ErrState
}

// TK returns the temporal key after message 4, or nil before.
func (h *Handshake) TK() []byte {
	if h.stage != hsDone {
		return nil
	}
	return h.keys.TK()
}

// Close zeroes and frees the keys of the station.
func (h *Handshake) Close() {
	h.keys.Close()
	clear(h.anonce[:])
	clear(h.snonce[:])
	if h.staRSN != nil {
		clear(h.staRSN)
		memory.FreeSlice(h.a.mem, h.staRSN)
		h.staRSN = nil
	}
	h.stage = hsIdle
}

// message1 writes message 1: the ANonce, no MIC and no key data.
func (h *Handshake) message1(reply []byte) (int, error) {
	h.replay++
	n, err := putKeyFrame(reply, 2, infoVersionAES|infoPairwise|infoAck, h.replay, h.anonce[:], nil)
	if err != nil {
		return 0, err
	}
	binary.BigEndian.PutUint16(reply[offKeyLen:], gtkLen)
	return n, nil
}

// message3 writes message 3: the RSN element of the access point and the GTK
// KDE, wrapped with the KEK (IEEE 802.11-2020 12.7.6.4).
func (h *Handshake) message3(reply []byte) (int, error) {
	h.replay++
	plainLen := len(h.a.rsn) + 2 + gtkKDEFixed + gtkLen
	if r := plainLen % 8; r != 0 || plainLen < 16 {
		plainLen += 8 - r
		if plainLen < 16 {
			plainLen = 16
		}
	}
	plain := memory.MakeSlice[byte](h.a.mem, plainLen)
	if plain == nil {
		return 0, ErrNoMemory
	}
	defer func() {
		clear(plain)
		memory.FreeSlice(h.a.mem, plain)
	}()
	k := copy(plain, h.a.rsn)
	k += copy(plain[k:], []byte{eidVendor, gtkKDEFixed + gtkLen, 0x00, 0x0f, 0xac, kdeGTK, gtkKeyID, 0})
	k += copy(plain[k:], h.a.gtk[:])
	if k < len(plain) {
		plain[k] = eidVendor // padding: 0xdd, then zeros
	}
	if offData+len(plain)+8 > len(reply) {
		return 0, ErrBuffer
	}
	wrapped := reply[offData : offData+len(plain)+8]
	if err := h.keys.WrapKey(plain, wrapped); err != nil {
		return 0, err
	}
	info := uint16(infoVersionAES | infoPairwise | infoAck | infoInstall | infoMIC | infoSecure | infoEncrypted)
	n, err := putKeyFrame(reply, 2, info, h.replay, h.anonce[:], wrapped)
	if err != nil {
		return 0, err
	}
	binary.BigEndian.PutUint16(reply[offKeyLen:], gtkLen)
	var mic [micLen]byte
	if err := h.keys.MIC(reply[:n], &mic); err != nil {
		return 0, err
	}
	copy(reply[offMIC:], mic[:])
	return n, nil
}

// verify checks the MIC of f on a scratch copy with a zero MIC field.
func (h *Handshake) verify(f *keyFrame) error {
	tmp := memory.MakeSlice[byte](h.a.mem, len(f.raw))
	if tmp == nil {
		return ErrNoMemory
	}
	copy(tmp, f.raw)
	clear(tmp[offMIC : offMIC+micLen])
	var mic [micLen]byte
	err := h.keys.MIC(tmp, &mic)
	ok := err == nil && subtle.ConstantTimeCompare(mic[:], f.raw[offMIC:offMIC+micLen]) == 1
	clear(mic[:])
	clear(tmp)
	memory.FreeSlice(h.a.mem, tmp)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMIC
	}
	return nil
}

// wpa2PSKCCMP reports whether rsn is an RSN element with the CCMP group
// cipher, and with CCMP and PSK in its lists (IEEE 802.11-2020 9.4.2.24).
func wpa2PSKCCMP(rsn []byte) bool {
	if len(rsn) < 2+2+4+2+4+2+4 || rsn[0] != 48 || int(rsn[1])+2 > len(rsn) {
		return false
	}
	b := rsn[2 : 2+int(rsn[1])]
	if len(b) < 2+4+2 || binary.LittleEndian.Uint16(b) != 1 || !suite(b[2:6], 4) {
		return false
	}
	b = b[6:]
	ccmp, b := list(b, 4)
	psk, _ := list(b, 2)
	return ccmp && psk
}

// list reports whether the suite list at the start of b has suite type t, and
// returns the rest of b.
func list(b []byte, t byte) (bool, []byte) {
	if len(b) < 2 {
		return false, nil
	}
	n := int(binary.LittleEndian.Uint16(b))
	b = b[2:]
	if len(b) < 4*n {
		return false, nil
	}
	found := false
	for i := range n {
		found = found || suite(b[4*i:4*i+4], t)
	}
	return found, b[4*n:]
}

func suite(s []byte, t byte) bool { return s[0] == 0x00 && s[1] == 0x0f && s[2] == 0xac && s[3] == t }
