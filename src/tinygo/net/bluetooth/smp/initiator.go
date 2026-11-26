package smp

import (
	"crypto/subtle"
	"io"

	"runtime/memory"
	"tinygo/crypto/p256"
)

const codeSecurityRequest = 0x0b

// Initiator runs LE Secure Connections pairing on the central side of one connection. After the
// DHKey check the host starts encryption with LTK, then calls Encrypted for key distribution.
type Initiator struct {
	endpoint
	securityRequest bool
}

// NewInitiator returns an initiator with the given IO capability. confirm may be nil, and then
// numeric comparison becomes Just Works.
func NewInitiator(mem Memory, rng io.Reader, send Sender, confirm Confirmer, ioCap uint8) (*Initiator, error) {
	if mem == nil || rng == nil || send == nil {
		return nil, ErrNilDependency
	}
	return &Initiator{endpoint: endpoint{mem: mem, rng: rng, send: send, confirm: confirm, ioCap: ioCap}}, nil
}

// SecurityRequested reports whether the peripheral sent a Security Request since Start (3.6.7).
// The host decides whether to call Pair.
func (i *Initiator) SecurityRequested() bool { return i.securityRequest }

// Pair sends the Pairing Request. With bonding, it asks for the identity keys of both sides.
func (i *Initiator) Pair(bonding bool, now uint32) error {
	if i.closed {
		return ErrClosed
	}
	i.drop()
	i.releaseBond()
	s := memory.New[session](i.mem)
	if s == nil {
		return ErrNoMemory
	}
	i.s, i.state, i.err = s, StatePairing, nil
	s.deadline = now + timeoutMs
	auth := uint8(authSC | authMITM)
	var initDist, respDist uint8
	if bonding {
		auth |= authBonding
		respDist = distIDKey
		if i.hasID {
			initDist = distIDKey
		}
	}
	s.preq = [7]byte{codePairingRequest, i.ioCap, 0, auth, maxKeySize, initDist, respDist}
	s.stage = stageWaitResponse
	copy(i.tx[:], s.preq[:])
	return i.send.SendSMP(i.tx[:7])
}

// Handle processes one SMP PDU from the responder.
func (i *Initiator) Handle(pdu []byte, now uint32) error {
	if i.closed {
		return ErrClosed
	}
	if len(pdu) == 0 {
		return nil
	}
	s := i.s
	if s != nil {
		s.deadline = now + timeoutMs
	}
	switch pdu[0] {
	case codeSecurityRequest:
		if len(pdu) == 2 {
			i.securityRequest = true
		}
		return nil
	case codePairingFailed:
		i.drop()
		i.releaseBond()
		i.state, i.err = StateFailed, ErrFailed
		return nil
	case codePairingResponse:
		if !i.expect(stageWaitResponse, pdu, 7) {
			return i.fail(ReasonUnspecified)
		}
		return i.pairingResponse(pdu)
	case codePublicKey:
		if !i.expect(stageWaitPublicKey, pdu, 65) {
			return i.fail(ReasonUnspecified)
		}
		return i.publicKey(pdu[1:])
	case codePairingConfirm:
		if !i.expect(stageWaitConfirm, pdu, 17) {
			return i.fail(ReasonUnspecified)
		}
		return i.confirmValue(pdu[1:])
	case codePairingRandom:
		if !i.expect(stageWaitRandom, pdu, 17) {
			return i.fail(ReasonUnspecified)
		}
		return i.random(pdu[1:])
	case codeDHKeyCheck:
		if !i.expect(stageWaitDHKeyCheck, pdu, 17) {
			return i.fail(ReasonUnspecified)
		}
		return i.dhKeyCheck(pdu[1:])
	case codeIdentityInfo:
		if !i.expect(stageWaitIdentityInfo, pdu, 17) {
			return i.fail(ReasonUnspecified)
		}
		i.takeIdentityInfo(pdu[1:])
		return nil
	case codeIdentityAddr:
		if !i.expect(stageWaitIdentityAddr, pdu, 8) {
			return i.fail(ReasonUnspecified)
		}
		if !i.takeIdentityAddr(pdu[1:]) {
			return i.fail(ReasonInvalidParameters)
		}
		return i.sendKeys()
	}
	return i.fail(ReasonCommandNotSupported)
}

// expect reports whether pdu has length n and pairing waits in stage st. A method and not a
// closure, because TinyGo moves the variables of a closure to the GC heap.
func (i *Initiator) expect(st stage, pdu []byte, n int) bool {
	return i.s != nil && i.s.stage == st && len(pdu) == n
}

// Encrypted tells the initiator that the link is encrypted with the LTK. The responder sends its
// keys first, then the initiator sends its own (3.6.1).
func (i *Initiator) Encrypted(now uint32) error {
	if i.closed {
		return ErrClosed
	}
	s := i.s
	if s == nil || s.stage != stageWaitEncryption {
		return nil
	}
	s.deadline = now + timeoutMs
	if s.pres[6]&distIDKey != 0 {
		s.stage = stageWaitIdentityInfo
		return nil
	}
	return i.sendKeys()
}

func (i *Initiator) pairingResponse(p []byte) error {
	s := i.s
	copy(s.pres[:], p)
	ioB, authB, keyB := p[1], p[3], p[4]
	if authB&authSC == 0 {
		return i.fail(ReasonAuthenticationRequirement) // Secure Connections Only mode (2.3.5.6)
	}
	if keyB < minKeySize || keyB > maxKeySize {
		return i.fail(ReasonEncryptionKeySize)
	}
	// The responder may only accept keys that the request asked for (3.6.1).
	if p[5]&^s.preq[5] != 0 || p[6]&^s.preq[6] != 0 {
		return i.fail(ReasonInvalidParameters)
	}
	m, ok := method(i.ioCap, ioB)
	if m == numericComparison && i.confirm == nil {
		m = justWorks
	}
	if !ok {
		return i.fail(ReasonPairingNotSupported)
	}
	if m >= passkeyShow {
		if i.showsPasskey(m) && i.display == nil || !i.showsPasskey(m) && i.input == nil {
			return i.fail(ReasonPairingNotSupported)
		}
	}
	s.method, s.keySize = m, keyB
	if err := p256.GenerateKey(i.mem, i.rng, &s.sk, &s.pkaX, &s.pkaY); err != nil {
		return i.fail(ReasonUnspecified)
	}
	s.stage = stageWaitPublicKey
	i.tx[0] = codePublicKey
	reverse(i.tx[1:33], s.pkaX[:])
	reverse(i.tx[33:65], s.pkaY[:])
	return i.send.SendSMP(i.tx[:65])
}

// showsPasskey reports whether this initiator shows the passkey. The table names what the
// responder does. When the responder types, the initiator shows if it has a display.
func (i *Initiator) showsPasskey(m pairMethod) bool {
	return m == passkeyEnter && i.ioCap != IOKeyboardOnly
}

func (i *Initiator) publicKey(p []byte) error {
	s := i.s
	reverse(s.pkbX[:], p[0:32])
	reverse(s.pkbY[:], p[32:64])
	if !p256.OnCurve(&s.pkbX, &s.pkbY) || s.pkbX == s.pkaX {
		return i.fail(ReasonDHKeyCheckFailed) // invalid or reflected public key
	}
	if err := p256.SharedSecret(i.mem, &s.sk, &s.pkbX, &s.pkbY, &s.dhKey); err != nil {
		return i.fail(ReasonDHKeyCheckFailed)
	}
	clear(s.sk[:])
	s.stage = stageWaitConfirm
	if s.method < passkeyShow {
		return nil // the responder sends its confirm value first
	}
	if i.showsPasskey(s.method) {
		if !randomPasskey(i.rng, &s.passkey) {
			return i.fail(ReasonUnspecified)
		}
		i.display.ShowPasskey(s.passkey)
	} else {
		v, ok := i.input.EnterPasskey()
		if !ok || v > 999999 {
			return i.fail(ReasonPasskeyEntryFailed)
		}
		s.passkey = v
	}
	s.round = 0
	return i.sendRoundConfirm()
}

// sendRoundConfirm starts a passkey round: a new Na and its confirm value, which the initiator
// sends first (2.3.5.6.3).
func (i *Initiator) sendRoundConfirm() error {
	s := i.s
	if _, err := io.ReadFull(i.rng, s.na[:]); err != nil {
		return i.fail(ReasonUnspecified)
	}
	var ca [16]byte
	if err := F4(i.mem, &s.pkaX, &s.pkbX, &s.na, s.passkeyBit(), &ca); err != nil {
		return i.fail(ReasonUnspecified)
	}
	s.stage = stageWaitConfirm
	return i.send16(codePairingConfirm, &ca)
}

// confirmValue keeps the confirm value of the responder and sends Na.
func (i *Initiator) confirmValue(p []byte) error {
	s := i.s
	reverse(s.peerConfirm[:], p)
	if s.method < passkeyShow {
		if _, err := io.ReadFull(i.rng, s.na[:]); err != nil {
			return i.fail(ReasonUnspecified)
		}
	}
	s.stage = stageWaitRandom
	return i.send16(codePairingRandom, &s.na)
}

// random checks Nb against the confirm value of the responder, then runs the next passkey round,
// or the numeric comparison, key derivation and the DHKey check of this side.
func (i *Initiator) random(p []byte) error {
	s := i.s
	reverse(s.nb[:], p)
	var z uint8
	if s.method >= passkeyShow {
		z = s.passkeyBit()
	}
	var cb [16]byte
	if err := F4(i.mem, &s.pkbX, &s.pkaX, &s.nb, z, &cb); err != nil {
		return i.fail(ReasonUnspecified)
	}
	if subtle.ConstantTimeCompare(cb[:], s.peerConfirm[:]) != 1 {
		return i.fail(ReasonConfirmValueFailed)
	}
	if s.method >= passkeyShow {
		s.round++
		if s.round < passkeyRounds {
			return i.sendRoundConfirm()
		}
	}
	if s.method == numericComparison {
		v, err := G2(i.mem, &s.pkaX, &s.pkbX, &s.na, &s.nb)
		if err != nil || !i.confirm.Confirm(v%1000000) {
			return i.fail(ReasonNumericComparisonFailed)
		}
	}
	a, b := i.own.spec(), i.peer.spec()
	if err := F5(i.mem, &s.dhKey, &s.na, &s.nb, &a, &b, &s.macKey, &s.ltk); err != nil {
		return i.fail(ReasonUnspecified)
	}
	clear(s.dhKey[:])
	var rv, ea [16]byte
	s.checkValue(&rv)
	ioA := [3]byte{s.preq[3], s.preq[2], s.preq[1]} // AuthReq, OOB flag, IO capability
	if err := F6(i.mem, &s.macKey, &s.na, &s.nb, &rv, &ioA, &a, &b, &ea); err != nil {
		return i.fail(ReasonUnspecified)
	}
	s.stage = stageWaitDHKeyCheck
	return i.send16(codeDHKeyCheck, &ea)
}

// dhKeyCheck checks Eb and keeps the LTK. The host then starts encryption.
func (i *Initiator) dhKeyCheck(p []byte) error {
	s := i.s
	var eb, want, rv [16]byte
	reverse(eb[:], p)
	s.checkValue(&rv)
	a, b := i.own.spec(), i.peer.spec()
	ioB := [3]byte{s.pres[3], s.pres[2], s.pres[1]}
	if err := F6(i.mem, &s.macKey, &s.nb, &s.na, &rv, &ioB, &b, &a, &want); err != nil {
		return i.fail(ReasonUnspecified)
	}
	if subtle.ConstantTimeCompare(eb[:], want[:]) != 1 {
		return i.fail(ReasonDHKeyCheckFailed)
	}
	bond := memory.New[Bond](i.mem)
	if bond == nil {
		return i.fail(ReasonUnspecified)
	}
	// Keys shorter than 16 bytes are masked to the negotiated size (2.3.4).
	reverse(bond.LTK[:], s.ltk[:])
	clear(bond.LTK[s.keySize:])
	bond.KeySize = s.keySize
	bond.Bonded = s.preq[3]&s.pres[3]&authBonding != 0
	i.bond = bond
	clear(s.ltk[:])
	clear(s.macKey[:])
	s.stage, i.state = stageWaitEncryption, StateDistributing
	return nil
}

// sendKeys sends the identity keys of this device when the responder accepted them, and ends pairing.
func (i *Initiator) sendKeys() error {
	if i.s.pres[5]&distIDKey != 0 {
		if err := i.sendIdentity(); err != nil {
			return err
		}
	}
	i.complete()
	return nil
}

// Start prepares pairing on a new connection between own (this device) and peer.
func (i *Initiator) Start(own, peer Address) {
	i.endpoint.Start(own, peer)
	i.securityRequest = false
}
