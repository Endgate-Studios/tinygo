package eapol

import (
	"crypto/subtle"
	"io"

	"runtime/memory"
	"tinygo/net/wifi/ieee80211"
	"tinygo/net/wifi/wpa"
)

// Memory is where the supplicant allocates key material and scratch buffers.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Event tells the caller what Handle changed.
type Event uint8

const (
	EventNone     Event = iota
	EventPairwise       // 4-way handshake done, install TK and GTK
	EventGroup          // group handshake done, install the new GTK
)

type stage uint8

const (
	stageWaitM1 stage = iota
	stageWaitM3
	stageDone
)

// Supplicant runs the station side of WPA2-Personal for one network.
type Supplicant struct {
	mem       Memory
	rng       io.Reader
	keys      *wpa.Keys
	aa, spa   ieee80211.MAC
	ownRSN    []byte
	apRSN     []byte
	gtk       []byte
	gtkLen    int
	gtkID     uint8
	gtkRSC    [rscLen]byte
	igtk      [2 + ipnLen + igtkLen]byte // key ID, IPN and IGTK
	hasIGTK   bool
	sae       bool
	anonce    [nonceLen]byte
	snonce    [nonceLen]byte
	replay    uint64
	replaySet bool
	stage     stage
	started   bool
	closed    bool
}

// New derives the PMK for passphrase and ssid, with all key material in mem.
// rng supplies the SNonce, so use the hardware random number generator on a device.
func New(mem Memory, rng io.Reader, passphrase, ssid []byte) (*Supplicant, error) {
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
	if err := keys.SetPassphrase(passphrase, ssid); err != nil {
		keys.Close()
		return nil, err
	}
	return &Supplicant{mem: mem, rng: rng, keys: keys}, nil
}

// NewPSK is New with the 32-byte pre-shared key in place of a passphrase.
func NewPSK(mem Memory, rng io.Reader, psk []byte) (*Supplicant, error) {
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
	if err := keys.SetPMK(psk); err != nil {
		keys.Close()
		return nil, err
	}
	return &Supplicant{mem: mem, rng: rng, keys: keys}, nil
}

// NewSAE is NewPSK with the PMK of SAE (WPA3-Personal). The 4-way handshake then uses
// key descriptor version 0: KDF-SHA256 for the PTK and AES-128-CMAC for the MIC.
func NewSAE(mem Memory, rng io.Reader, pmk []byte) (*Supplicant, error) {
	s, err := NewPSK(mem, rng, pmk)
	if err != nil {
		return nil, err
	}
	s.sae = true
	s.keys.UseSAE()
	return s, nil
}

// version gives the key descriptor version of the AKM.
func (s *Supplicant) version() uint16 {
	if s.sae {
		return infoVersionAKM
	}
	return infoVersionAES
}

// Start prepares a new association with access point aa from station address spa.
// ownRSN is the full RSN element of the association request. apRSN is the full RSN element
// of the beacon, or nil to skip the downgrade check of message 3.
func (s *Supplicant) Start(aa, spa ieee80211.MAC, ownRSN, apRSN []byte) error {
	if s.closed {
		return ErrClosed
	}
	s.releaseAssoc()
	s.aa, s.spa = aa, spa
	var err error
	if s.ownRSN, err = s.copyOf(ownRSN); err != nil {
		return err
	}
	if apRSN != nil {
		if s.apRSN, err = s.copyOf(apRSN); err != nil {
			return err
		}
	}
	s.stage, s.replaySet, s.replay, s.started = stageWaitM1, false, 0, true
	return nil
}

// Handle processes one EAPOL frame from the access point, starting at the EAPOL header.
// When it returns n > 0, send reply[:n] to the access point as an EAPOL frame.
func (s *Supplicant) Handle(frame, reply []byte) (int, Event, error) {
	if s.closed {
		return 0, EventNone, ErrClosed
	}
	if !s.started {
		return 0, EventNone, ErrNotStarted
	}
	f, err := parseKeyFrame(frame, s.version())
	if err != nil {
		return 0, EventNone, err
	}
	if f.info&infoAck == 0 || f.info&(infoRequest|infoError) != 0 {
		return 0, EventNone, ErrState
	}
	if s.replaySet && f.replay <= s.replay {
		return 0, EventNone, ErrReplay
	}
	switch {
	case f.info&infoPairwise != 0 && f.info&infoMIC == 0:
		return s.message1(&f, reply)
	case f.info&infoPairwise != 0:
		return s.message3(&f, reply)
	default:
		return s.group1(&f, reply)
	}
}

// TK returns the temporal key after the 4-way handshake, or nil before.
func (s *Supplicant) TK() []byte {
	if s.stage != stageDone {
		return nil
	}
	return s.keys.TK()
}

// GTK returns the group key and its key ID after the 4-way handshake, or nil before.
func (s *Supplicant) GTK() ([]byte, uint8) {
	if s.stage != stageDone {
		return nil, 0
	}
	return s.gtk[:s.gtkLen], s.gtkID
}

// IGTK returns the IGTK of PMF, its key ID and its IPN (the receive packet number, low byte
// first) after the 4-way handshake or a group handshake that gave one, or nil.
func (s *Supplicant) IGTK() (igtk []byte, id uint16, ipn []byte) {
	if s.stage != stageDone || !s.hasIGTK {
		return nil, 0, nil
	}
	return s.igtk[2+ipnLen:], uint16(s.igtk[0]) | uint16(s.igtk[1])<<8, s.igtk[2 : 2+ipnLen]
}

// GTKRSC returns the receive sequence counter of the group key from message 3 or
// group message 1, or nil before the 4-way handshake. The first 6 bytes are the
// CCMP packet number, low byte first.
func (s *Supplicant) GTKRSC() []byte {
	if s.stage != stageDone {
		return nil
	}
	return s.gtkRSC[:]
}

// Close zeroes and frees all key material. The Supplicant cannot be used after Close.
func (s *Supplicant) Close() {
	s.releaseAssoc()
	s.keys.Close()
	clear(s.anonce[:])
	clear(s.snonce[:])
	s.closed = true
}

func (s *Supplicant) message1(f *keyFrame, reply []byte) (int, Event, error) {
	copy(s.anonce[:], f.nonce)
	if _, err := io.ReadFull(s.rng, s.snonce[:]); err != nil {
		return 0, EventNone, ErrRandom
	}
	if err := s.keys.DerivePTK(s.aa, s.spa, s.anonce, s.snonce); err != nil {
		return 0, EventNone, err
	}
	n, err := putKeyFrame(reply, f.version, s.version()|infoPairwise|infoMIC, f.replay, s.snonce[:], s.ownRSN)
	if err != nil {
		return 0, EventNone, err
	}
	if err := s.sign(reply[:n]); err != nil {
		return 0, EventNone, err
	}
	s.stage = stageWaitM3
	return n, EventNone, nil
}

func (s *Supplicant) message3(f *keyFrame, reply []byte) (int, Event, error) {
	if s.stage != stageWaitM3 {
		return 0, EventNone, ErrState
	}
	if f.info&(infoInstall|infoEncrypted) != infoInstall|infoEncrypted {
		return 0, EventNone, ErrState
	}
	if err := s.verify(f); err != nil {
		return 0, EventNone, err
	}
	if subtle.ConstantTimeCompare(f.nonce, s.anonce[:]) != 1 {
		return 0, EventNone, ErrNonce
	}
	s.replay, s.replaySet = f.replay, true
	if err := s.readKeyData(f.data, true); err != nil {
		return 0, EventNone, err
	}
	copy(s.gtkRSC[:], f.rsc)
	n, err := putKeyFrame(reply, f.version, s.version()|infoPairwise|infoMIC|infoSecure, f.replay, nil, nil)
	if err != nil {
		return 0, EventNone, err
	}
	if err := s.sign(reply[:n]); err != nil {
		return 0, EventNone, err
	}
	s.stage = stageDone
	return n, EventPairwise, nil
}

func (s *Supplicant) group1(f *keyFrame, reply []byte) (int, Event, error) {
	if s.stage != stageDone {
		return 0, EventNone, ErrState
	}
	if f.info&(infoMIC|infoEncrypted) != infoMIC|infoEncrypted {
		return 0, EventNone, ErrState
	}
	if err := s.verify(f); err != nil {
		return 0, EventNone, err
	}
	s.replay = f.replay
	if err := s.readKeyData(f.data, false); err != nil {
		return 0, EventNone, err
	}
	copy(s.gtkRSC[:], f.rsc)
	n, err := putKeyFrame(reply, f.version, s.version()|infoMIC|infoSecure, f.replay, nil, nil)
	if err != nil {
		return 0, EventNone, err
	}
	if err := s.sign(reply[:n]); err != nil {
		return 0, EventNone, err
	}
	return n, EventGroup, nil
}

// verify checks the MIC of f on a scratch copy with a zero MIC field.
func (s *Supplicant) verify(f *keyFrame) error {
	tmp := memory.MakeSlice[byte](s.mem, len(f.raw))
	if tmp == nil {
		return ErrNoMemory
	}
	copy(tmp, f.raw)
	clear(tmp[offMIC : offMIC+micLen])
	var mic [micLen]byte
	err := s.keys.MIC(tmp, &mic)
	ok := err == nil && subtle.ConstantTimeCompare(mic[:], f.raw[offMIC:offMIC+micLen]) == 1
	clear(mic[:])
	s.drop(tmp)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMIC
	}
	return nil
}

// sign writes the MIC into a reply frame whose MIC field is zero.
func (s *Supplicant) sign(frame []byte) error {
	var mic [micLen]byte
	if err := s.keys.MIC(frame, &mic); err != nil {
		return err
	}
	copy(frame[offMIC:], mic[:])
	clear(mic[:])
	return nil
}

// readKeyData unwraps the key data, checks the RSN element of message 3 and keeps the GTK.
func (s *Supplicant) readKeyData(wrapped []byte, checkRSN bool) error {
	if len(wrapped) < 24 || len(wrapped)%8 != 0 {
		return ErrKeyData
	}
	plain := memory.MakeSlice[byte](s.mem, len(wrapped)-8)
	if plain == nil {
		return ErrNoMemory
	}
	defer s.drop(plain)
	if err := s.keys.UnwrapKey(wrapped, plain); err != nil {
		return err
	}
	if checkRSN && s.apRSN != nil {
		got := findRSN(plain)
		if got == nil || subtle.ConstantTimeCompare(got, s.apRSN) != 1 {
			return ErrRSNMismatch
		}
	}
	gtk, id, err := findGTK(plain)
	if err != nil {
		return err
	}
	if s.gtk == nil {
		if s.gtk = memory.MakeSlice[byte](s.mem, maxGTKLen); s.gtk == nil {
			return ErrNoMemory
		}
	}
	clear(s.gtk)
	s.gtkLen = copy(s.gtk, gtk)
	s.gtkID = id
	if b := findIGTK(plain); b != nil {
		copy(s.igtk[:], b)
		s.hasIGTK = true
	}
	return nil
}

func (s *Supplicant) copyOf(b []byte) ([]byte, error) {
	c := memory.MakeSlice[byte](s.mem, len(b))
	if c == nil && len(b) != 0 {
		return nil, ErrNoMemory
	}
	copy(c, b)
	return c, nil
}

func (s *Supplicant) drop(b []byte) {
	clear(b)
	memory.FreeSlice(s.mem, b)
}

// releaseAssoc frees the state of the current association.
func (s *Supplicant) releaseAssoc() {
	s.release(&s.ownRSN)
	s.release(&s.apRSN)
	s.release(&s.gtk)
	s.gtkLen, s.gtkID, s.started = 0, 0, false
	clear(s.gtkRSC[:])
	clear(s.igtk[:])
	s.hasIGTK = false
}

func (s *Supplicant) release(b *[]byte) {
	if *b != nil {
		s.drop(*b)
		*b = nil
	}
}
