package smp

import (
	"io"
	"unsafe"
)

// endpoint is the state and the helpers that the initiator and the responder share.
type endpoint struct {
	mem     Memory
	rng     io.Reader
	send    Sender
	confirm Confirmer
	display PasskeyDisplay
	input   PasskeyInput
	ioCap   uint8
	own     Address
	peer    Address
	s       *session
	bond    *Bond // from the LTK until Close or the next pairing
	irk     [16]byte
	id      Address
	hasID   bool
	tx      [65]byte // outgoing PDU, kept here so it does not escape through Sender
	state   State
	err     error
	closed  bool
}

// SetIdentity sets the IRK in wire order and the identity address that this device distributes.
// Without it, this device does not distribute an identity.
func (r *endpoint) SetIdentity(irk *[16]byte, identity Address) {
	r.irk, r.id, r.hasID = *irk, identity, true
}

// SetPasskeyIO sets how this device shows or enters a passkey. Either may be nil, and then the
// pairings that need it fail with Pairing Not Supported.
func (r *endpoint) SetPasskeyIO(display PasskeyDisplay, input PasskeyInput) {
	r.display, r.input = display, input
}

// Start prepares pairing on a new connection between own (this device) and peer.
func (r *endpoint) Start(own, peer Address) {
	r.drop()
	r.releaseBond()
	r.own, r.peer, r.state, r.err = own, peer, StateIdle, nil
}

// State returns the pairing state, and Err why pairing failed.
func (r *endpoint) State() State { return r.state }

func (r *endpoint) Err() error { return r.err }

// LTK copies the long term key in wire order for HCI_LE_Long_Term_Key_Request_Reply.
// It reports false before the DHKey check is complete.
func (r *endpoint) LTK(dst *[16]byte) bool {
	if r.bond == nil {
		return false
	}
	*dst = r.bond.LTK
	return true
}

// Bond copies the result of pairing, and reports false before pairing is complete.
func (r *endpoint) Bond(dst *Bond) bool {
	if r.bond == nil || r.state != StatePaired {
		return false
	}
	*dst = *r.bond
	return true
}

// Close zeroes and frees all pairing state and the bond, and zeroes the IRK.
func (r *endpoint) Close() {
	r.drop()
	r.releaseBond()
	clear(r.irk[:])
	r.closed = true
}

// Tick ends pairing when the peer is silent for the SMP timeout of 30 seconds.
func (r *endpoint) Tick(now uint32) error {
	if r.closed {
		return ErrClosed
	}
	if r.s != nil && int32(now-r.s.deadline) >= 0 {
		r.drop()
		r.releaseBond()
		r.state, r.err = StateFailed, ErrTimeout
	}
	return nil
}

// complete ends pairing with success and keeps only the bond.
func (r *endpoint) complete() {
	r.drop()
	r.state = StatePaired
}

// send16 sends a command with a 16-byte value in wire order.
func (r *endpoint) send16(code byte, v *[16]byte) error {
	r.tx[0] = code
	reverse(r.tx[1:17], v[:])
	return r.send.SendSMP(r.tx[:17])
}

// fail sends Pairing Failed and drops the session.
func (r *endpoint) fail(reason uint8) error {
	r.drop()
	r.releaseBond()
	r.state, r.err = StateFailed, ErrFailed
	r.tx[0], r.tx[1] = codePairingFailed, reason
	return r.send.SendSMP(r.tx[:2])
}

func (r *endpoint) drop() {
	if r.s == nil {
		return
	}
	*r.s = session{}
	r.mem.Free(unsafe.Pointer(r.s))
	r.s = nil
}

func (r *endpoint) releaseBond() {
	if r.bond != nil {
		*r.bond = Bond{}
		r.mem.Free(unsafe.Pointer(r.bond))
		r.bond = nil
	}
}

// sendIdentity sends the IRK and the identity address of this device (3.6.4 and 3.6.5).
func (r *endpoint) sendIdentity() error {
	r.tx[0] = codeIdentityInfo
	copy(r.tx[1:17], r.irk[:])
	if err := r.send.SendSMP(r.tx[:17]); err != nil {
		return err
	}
	r.tx[0], r.tx[1] = codeIdentityAddr, r.id.Type&1
	copy(r.tx[2:8], r.id.Addr[:])
	return r.send.SendSMP(r.tx[:8])
}

// takeIdentityInfo keeps the IRK of the peer, and waits for its identity address.
func (r *endpoint) takeIdentityInfo(irk []byte) {
	copy(r.bond.PeerIRK[:], irk)
	r.s.stage = stageWaitIdentityAddr
}

// takeIdentityAddr keeps the identity address of the peer. It reports false when the address is
// not public and not static random, which has the top two bits set (Vol 6 Part B 1.3.2.1).
func (r *endpoint) takeIdentityAddr(p []byte) bool {
	if p[0] > 1 || (p[0] == 1 && p[6]>>6 != 0b11) {
		return false
	}
	r.bond.PeerIdentity.Type = p[0]
	copy(r.bond.PeerIdentity.Addr[:], p[1:7])
	r.bond.HasPeerIRK = true
	return true
}

// checkValue sets v to the r value of the DHKey check: the passkey for passkey entry, and zero for
// the other models (2.3.5.6.5).
func (s *session) checkValue(v *[16]byte) {
	clear(v[:])
	if s.method >= passkeyShow {
		v[12], v[13], v[14], v[15] = byte(s.passkey>>24), byte(s.passkey>>16), byte(s.passkey>>8), byte(s.passkey)
	}
}
