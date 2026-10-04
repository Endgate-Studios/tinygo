package smp

import (
	"crypto/subtle"
	"io"

	"runtime/memory"
	"tinygo/net/bluetooth/p256"
)

// SMP command codes (Core spec v5.4 Vol 3 Part H 3.3).
const (
	codePairingRequest  = 0x01
	codePairingResponse = 0x02
	codePairingConfirm  = 0x03
	codePairingRandom   = 0x04
	codePairingFailed   = 0x05
	codePublicKey       = 0x0c
	codeDHKeyCheck      = 0x0d
	codeIdentityInfo    = 0x08
	codeIdentityAddr    = 0x09
)

// distIDKey is the IdKey bit of the key distribution fields (3.6.1). Secure Connections derives
// the LTK, so EncKey is not distributed, and SignKey and LinkKey are not supported.
const distIDKey = 0x02

// Pairing Failed reason codes (3.5.5).
const (
	ReasonPasskeyEntryFailed        = 0x01
	ReasonConfirmValueFailed        = 0x04
	ReasonPairingNotSupported       = 0x05
	ReasonEncryptionKeySize         = 0x06
	ReasonCommandNotSupported       = 0x07
	ReasonUnspecified               = 0x08
	ReasonInvalidParameters         = 0x0a
	ReasonDHKeyCheckFailed          = 0x0b
	ReasonNumericComparisonFailed   = 0x0c
	ReasonAuthenticationRequirement = 0x03
)

// IO capabilities (3.5.1).
const (
	IODisplayOnly     = 0x00
	IODisplayYesNo    = 0x01
	IOKeyboardOnly    = 0x02
	IONoInputNoOutput = 0x03
	IOKeyboardDisplay = 0x04
)

// AuthReq bits (3.5.1).
const (
	authBonding = 0x01
	authMITM    = 0x04
	authSC      = 0x08
)

const (
	timeoutMs  = 30000 // SMP timeout (3.4)
	minKeySize = 7
	maxKeySize = 16
)

// Errors of the responder.
const (
	ErrNilDependency Error = "smp: memory, random source and sender cannot be nil"
	ErrFailed        Error = "smp: pairing failed"
	ErrTimeout       Error = "smp: pairing timed out"
	ErrClosed        Error = "smp: responder is closed"
	ErrRandom        Error = "smp: random source failed"
)

// Address is an LE device address: type 0 public or 1 random, and the address in wire order.
type Address struct {
	Type uint8
	Addr [6]byte
}

// spec returns the 56-bit address of f5 and f6: type, then the address most significant byte first.
func (a Address) spec() [7]byte {
	var b [7]byte
	b[0] = a.Type & 1
	for i := 0; i < 6; i++ {
		b[1+i] = a.Addr[5-i]
	}
	return b
}

// Bond is what pairing gives for later connections. Keys and the address are in wire order.
type Bond struct {
	LTK          [16]byte // masked to KeySize
	PeerIRK      [16]byte // valid when HasPeerIRK
	PeerIdentity Address  // valid when HasPeerIRK
	KeySize      uint8
	HasPeerIRK   bool
	Bonded       bool // both sides set the Bonding flag, so the keys may be stored
}

// Sender sends one SMP PDU, for example through l2cap.Send on CIDSMP.
type Sender interface {
	SendSMP(pdu []byte) error
}

// Confirmer shows the 6-digit numeric comparison value to the user and returns the answer.
// Handle waits for it, so it must answer before the SMP timeout of 30 seconds.
type Confirmer interface {
	Confirm(value uint32) bool
}

// PasskeyDisplay shows the 6-digit passkey that the user types on the initiator.
type PasskeyDisplay interface {
	ShowPasskey(passkey uint32)
}

// PasskeyInput asks the user for the passkey that the initiator shows. ok is false when the user
// cancels. Handle waits for it, so it must answer before the SMP timeout of 30 seconds.
type PasskeyInput interface {
	EnterPasskey() (passkey uint32, ok bool)
}

// pairMethod is the association model (Core spec v5.4 Vol 3 Part H 2.3.5.1).
type pairMethod uint8

const (
	justWorks pairMethod = iota
	numericComparison
	passkeyShow  // this device shows the passkey and the initiator types it
	passkeyEnter // this device types the passkey
)

// passkeyRounds is one round for each bit of the 20-bit passkey (2.3.5.6.3).
const passkeyRounds = 20

// State is the pairing state.
type State uint8

const (
	StateIdle State = iota
	StatePairing
	StatePaired
	StateFailed
	// StateDistributing means the LTK is ready, and key distribution waits for encryption or runs.
	StateDistributing
)

type stage uint8

const (
	stageWaitPublicKey stage = iota
	stageWaitResponse
	stageWaitConfirm
	stageWaitRandom
	stageWaitDHKeyCheck
	stageWaitEncryption
	stageWaitIdentityInfo
	stageWaitIdentityAddr
)

// session is the pairing state of one attempt. It holds secrets, so it lives in the injected memory
// and is zeroed before it is freed. Values are in the spec order, most significant byte first.
type session struct {
	preq, pres          [7]byte
	sk, pkbX, pkbY      [32]byte
	pkaX, pkaY, dhKey   [32]byte
	na, nb, macKey, ltk [16]byte
	stage               stage
	method              pairMethod
	round               uint8
	passkey             uint32
	peerConfirm         [16]byte // confirm value of the peer, checked when its random value comes
	keySize             uint8
	deadline            uint32
}

// NewResponder returns a responder with the given IO capability. confirm may be nil when ioCap has
// no display with yes and no, and then pairing always uses Just Works.
// Responder runs LE Secure Connections pairing on the peripheral side of one connection.
type Responder struct {
	endpoint
}

func NewResponder(mem Memory, rng io.Reader, send Sender, confirm Confirmer, ioCap uint8) (*Responder, error) {
	if mem == nil || rng == nil || send == nil {
		return nil, ErrNilDependency
	}
	return &Responder{endpoint{mem: mem, rng: rng, send: send, confirm: confirm, ioCap: ioCap}}, nil
}

// Encrypted tells the responder that the link is encrypted with the LTK, after the HCI Encryption
// Change event. Key distribution starts then (3.6.1), and the responder sends its keys first.
func (r *Responder) Encrypted(now uint32) error {
	if r.closed {
		return ErrClosed
	}
	s := r.s
	if s == nil || s.stage != stageWaitEncryption {
		return nil
	}
	s.deadline = now + timeoutMs
	if s.pres[6]&distIDKey != 0 {
		if err := r.sendIdentity(); err != nil {
			return err
		}
	}
	if s.pres[5]&distIDKey != 0 {
		s.stage = stageWaitIdentityInfo
		return nil
	}
	r.complete()
	return nil
}

// Handle processes one SMP PDU from the initiator.
func (r *Responder) Handle(pdu []byte, now uint32) error {
	if r.closed {
		return ErrClosed
	}
	if len(pdu) == 0 {
		return nil
	}
	if r.s != nil {
		r.s.deadline = now + timeoutMs
	}
	switch pdu[0] {
	case codePairingRequest:
		return r.pairingRequest(pdu, now)
	case codePairingFailed:
		r.drop()
		r.releaseBond()
		r.state, r.err = StateFailed, ErrFailed
		return nil
	case codePublicKey:
		if r.s == nil || r.s.stage != stageWaitPublicKey || len(pdu) != 65 {
			return r.fail(ReasonUnspecified)
		}
		return r.publicKey(pdu[1:])
	case codePairingRandom:
		if r.s == nil || r.s.stage != stageWaitRandom || len(pdu) != 17 {
			return r.fail(ReasonUnspecified)
		}
		return r.random(pdu[1:])
	case codeDHKeyCheck:
		if r.s == nil || r.s.stage != stageWaitDHKeyCheck || len(pdu) != 17 {
			return r.fail(ReasonUnspecified)
		}
		return r.dhKeyCheck(pdu[1:])
	case codePairingConfirm:
		if r.s == nil || r.s.stage != stageWaitConfirm || len(pdu) != 17 {
			return r.fail(ReasonUnspecified)
		}
		return r.passkeyConfirm(pdu[1:])
	case codeIdentityInfo:
		if r.s == nil || r.s.stage != stageWaitIdentityInfo || len(pdu) != 17 {
			return r.fail(ReasonUnspecified)
		}
		r.takeIdentityInfo(pdu[1:])
		return nil
	case codeIdentityAddr:
		if r.s == nil || r.s.stage != stageWaitIdentityAddr || len(pdu) != 8 {
			return r.fail(ReasonUnspecified)
		}
		if !r.takeIdentityAddr(pdu[1:]) {
			return r.fail(ReasonInvalidParameters)
		}
		r.complete()
		return nil
	}
	return r.fail(ReasonCommandNotSupported)
}

func (r *Responder) pairingRequest(pdu []byte, now uint32) error {
	if len(pdu) != 7 {
		return r.fail(ReasonInvalidParameters)
	}
	r.drop()
	r.releaseBond()
	s := memory.New[session](r.mem)
	if s == nil {
		return ErrNoMemory
	}
	r.s, r.state = s, StatePairing
	s.deadline = now + timeoutMs
	copy(s.preq[:], pdu)
	ioA, authA, keyA := pdu[1], pdu[3], pdu[4]
	if authA&authSC == 0 {
		return r.fail(ReasonAuthenticationRequirement) // Secure Connections Only mode (2.3.5.6)
	}
	if keyA < minKeySize {
		return r.fail(ReasonEncryptionKeySize)
	}
	m, ok := method(ioA, r.ioCap)
	if m == numericComparison && r.confirm == nil {
		m = justWorks
	}
	if !ok || m == passkeyShow && r.display == nil || m == passkeyEnter && r.input == nil {
		return r.fail(ReasonPairingNotSupported)
	}
	s.method = m
	s.keySize = min(keyA, maxKeySize)
	auth := authA&authBonding | authSC
	if m != justWorks {
		auth |= authMITM
	}
	// Pairing Response: IO capability, no OOB data, AuthReq, key size, and the identity keys of
	// each side when the initiator asks for them.
	respDist := uint8(0)
	if r.hasID {
		respDist = pdu[6] & distIDKey
	}
	s.pres = [7]byte{codePairingResponse, r.ioCap, 0, auth, maxKeySize, pdu[5] & distIDKey, respDist}
	copy(r.tx[:], s.pres[:])
	if err := r.send.SendSMP(r.tx[:7]); err != nil {
		return err
	}
	if err := p256.GenerateKey(r.mem, r.rng, &s.sk, &s.pkbX, &s.pkbY); err != nil {
		return r.fail(ReasonUnspecified)
	}
	s.stage = stageWaitPublicKey
	return nil
}

// methods is the Secure Connections column of table 2.8 (2.3.5.1). The row is the IO capability of
// the initiator and the column is the IO capability of this responder.
var methods = [5][5]pairMethod{
	IODisplayOnly:     {justWorks, justWorks, passkeyEnter, justWorks, passkeyEnter},
	IODisplayYesNo:    {justWorks, numericComparison, passkeyEnter, justWorks, numericComparison},
	IOKeyboardOnly:    {passkeyShow, passkeyShow, passkeyEnter, justWorks, passkeyShow},
	IONoInputNoOutput: {justWorks, justWorks, justWorks, justWorks, justWorks},
	IOKeyboardDisplay: {passkeyShow, numericComparison, passkeyEnter, justWorks, numericComparison},
}

// method picks the association model from the IO capabilities, and reports false for values
// that are not defined.
func method(a, b uint8) (pairMethod, bool) {
	if a > IOKeyboardDisplay || b > IOKeyboardDisplay {
		return justWorks, false
	}
	return methods[a][b], true
}

func (r *Responder) publicKey(p []byte) error {
	s := r.s
	reverse(s.pkaX[:], p[0:32])
	reverse(s.pkaY[:], p[32:64])
	if !p256.OnCurve(&s.pkaX, &s.pkaY) || s.pkaX == s.pkbX {
		return r.fail(ReasonDHKeyCheckFailed) // invalid or reflected public key
	}
	if err := p256.SharedSecret(r.mem, &s.sk, &s.pkaX, &s.pkaY, &s.dhKey); err != nil {
		return r.fail(ReasonDHKeyCheckFailed)
	}
	clear(s.sk[:])
	r.tx[0] = codePublicKey
	reverse(r.tx[1:33], s.pkbX[:])
	reverse(r.tx[33:65], s.pkbY[:])
	if err := r.send.SendSMP(r.tx[:65]); err != nil {
		return err
	}
	if s.method >= passkeyShow {
		return r.startPasskey()
	}
	if _, err := io.ReadFull(r.rng, s.nb[:]); err != nil {
		return r.fail(ReasonUnspecified)
	}
	var cb [16]byte
	if err := F4(r.mem, &s.pkbX, &s.pkaX, &s.nb, 0, &cb); err != nil {
		return r.fail(ReasonUnspecified)
	}
	s.stage = stageWaitRandom
	return r.send16(codePairingConfirm, &cb)
}

// startPasskey gets the passkey, then waits for the first confirm value of the initiator, which
// sends first in each passkey round (2.3.5.6.3).
func (r *Responder) startPasskey() error {
	s := r.s
	if s.method == passkeyShow {
		if !randomPasskey(r.rng, &s.passkey) {
			return r.fail(ReasonUnspecified)
		}
		r.display.ShowPasskey(s.passkey)
	} else {
		v, ok := r.input.EnterPasskey()
		if !ok || v > 999999 {
			return r.fail(ReasonPasskeyEntryFailed)
		}
		s.passkey = v
	}
	s.round, s.stage = 0, stageWaitConfirm
	return nil
}

// randomPasskey draws a passkey from 0 to 999999 with no bias, from 20 random bits at a time.
func randomPasskey(rng io.Reader, dst *uint32) bool {
	var b [4]byte
	for tries := 0; tries < 32; tries++ {
		if _, err := io.ReadFull(rng, b[:]); err != nil {
			return false
		}
		v := (uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16) & 0xfffff
		if v < 1000000 {
			*dst = v
			clear(b[:])
			return true
		}
	}
	clear(b[:])
	return false
}

// passkeyBit is the f4 value z of this round: 0x80 with the passkey bit of the round, low bit first.
func (s *session) passkeyBit() uint8 { return 0x80 | uint8(s.passkey>>s.round&1) }

// passkeyConfirm keeps the confirm value of the initiator and answers with the confirm value of
// this round.
func (r *Responder) passkeyConfirm(p []byte) error {
	s := r.s
	reverse(s.peerConfirm[:], p)
	if _, err := io.ReadFull(r.rng, s.nb[:]); err != nil {
		return r.fail(ReasonUnspecified)
	}
	var cb [16]byte
	if err := F4(r.mem, &s.pkbX, &s.pkaX, &s.nb, s.passkeyBit(), &cb); err != nil {
		return r.fail(ReasonUnspecified)
	}
	s.stage = stageWaitRandom
	return r.send16(codePairingConfirm, &cb)
}

func (r *Responder) random(p []byte) error {
	s := r.s
	reverse(s.na[:], p)
	if s.method >= passkeyShow {
		var ca [16]byte
		if err := F4(r.mem, &s.pkaX, &s.pkbX, &s.na, s.passkeyBit(), &ca); err != nil {
			return r.fail(ReasonUnspecified)
		}
		if subtle.ConstantTimeCompare(ca[:], s.peerConfirm[:]) != 1 {
			return r.fail(ReasonConfirmValueFailed)
		}
	}
	if err := r.send16(codePairingRandom, &s.nb); err != nil {
		return err
	}
	if s.method >= passkeyShow {
		s.round++
		if s.round < passkeyRounds {
			s.stage = stageWaitConfirm
			return nil
		}
	}
	if s.method == numericComparison {
		v, err := G2(r.mem, &s.pkaX, &s.pkbX, &s.na, &s.nb)
		if err != nil || !r.confirm.Confirm(v%1000000) {
			return r.fail(ReasonNumericComparisonFailed)
		}
	}
	a, b := r.peer.spec(), r.own.spec()
	if err := F5(r.mem, &s.dhKey, &s.na, &s.nb, &a, &b, &s.macKey, &s.ltk); err != nil {
		return r.fail(ReasonUnspecified)
	}
	clear(s.dhKey[:])
	s.stage = stageWaitDHKeyCheck
	return nil
}

func (r *Responder) dhKeyCheck(p []byte) error {
	s := r.s
	var ea, want, rv [16]byte
	reverse(ea[:], p)
	s.checkValue(&rv)
	a, b := r.peer.spec(), r.own.spec()
	ioA := [3]byte{s.preq[3], s.preq[2], s.preq[1]} // AuthReq, OOB flag, IO capability
	if err := F6(r.mem, &s.macKey, &s.na, &s.nb, &rv, &ioA, &a, &b, &want); err != nil {
		return r.fail(ReasonUnspecified)
	}
	if subtle.ConstantTimeCompare(ea[:], want[:]) != 1 {
		return r.fail(ReasonDHKeyCheckFailed)
	}
	ioB := [3]byte{s.pres[3], s.pres[2], s.pres[1]}
	var eb [16]byte
	if err := F6(r.mem, &s.macKey, &s.nb, &s.na, &rv, &ioB, &b, &a, &eb); err != nil {
		return r.fail(ReasonUnspecified)
	}
	if err := r.send16(codeDHKeyCheck, &eb); err != nil {
		return err
	}
	bond := memory.New[Bond](r.mem)
	if bond == nil {
		return r.fail(ReasonUnspecified)
	}
	// Keys shorter than 16 bytes are masked to the negotiated size (2.3.4).
	reverse(bond.LTK[:], s.ltk[:])
	clear(bond.LTK[s.keySize:])
	bond.KeySize = s.keySize
	bond.Bonded = s.pres[3]&authBonding != 0
	r.bond = bond
	clear(s.ltk[:])
	clear(s.macKey[:])
	if s.pres[5]|s.pres[6] == 0 {
		r.complete()
		return nil
	}
	s.stage, r.state = stageWaitEncryption, StateDistributing
	return nil
}

// reverse copies src into dst in reverse byte order, between wire order and spec order.
func reverse(dst, src []byte) {
	n := len(src)
	for i := 0; i < n; i++ {
		dst[i] = src[n-1-i]
	}
}
