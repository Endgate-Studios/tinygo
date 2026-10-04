package smp

import (
	"math/rand"
	"testing"
)

// pairLink connects an Initiator and a Responder through two queues.
type pairLink struct {
	t      *testing.T
	i      *Initiator
	r      *Responder
	toR    *queue
	toI    *queue
	imem   *checkedMemory
	rmem   *checkedMemory
	shared uint32 // the passkey that the responder showed, for the user at the initiator
}

type recordConfirm struct {
	values []uint32
	ok     bool
}

func (c *recordConfirm) Confirm(v uint32) bool {
	c.values = append(c.values, v)
	return c.ok
}

func newPairLink(t *testing.T, ioI, ioR uint8, ci, cr Confirmer) *pairLink {
	t.Helper()
	l := &pairLink{t: t, toR: &queue{}, toI: &queue{}, imem: newCheckedMemory(t), rmem: newCheckedMemory(t)}
	var err error
	if l.i, err = NewInitiator(l.imem, rand.New(rand.NewSource(31)), l.toR, ci, ioI); err != nil {
		t.Fatal(err)
	}
	if l.r, err = NewResponder(l.rmem, rand.New(rand.NewSource(37)), l.toI, cr, ioR); err != nil {
		t.Fatal(err)
	}
	l.i.Start(central, peripheral)
	l.r.Start(peripheral, central)
	return l
}

// pump moves PDUs between the two sides until both queues are empty.
func (l *pairLink) pump() {
	l.t.Helper()
	for n := 0; n < 1000 && (len(l.toR.pdus) > 0 || len(l.toI.pdus) > 0); n++ {
		if len(l.toR.pdus) > 0 {
			p := l.toR.pdus[0]
			l.toR.pdus = l.toR.pdus[1:]
			if err := l.r.Handle(p, 0); err != nil {
				l.t.Fatal(err)
			}
			continue
		}
		p := l.toI.pdus[0]
		l.toI.pdus = l.toI.pdus[1:]
		if err := l.i.Handle(p, 0); err != nil {
			l.t.Fatal(err)
		}
	}
}

// pair runs pairing and, when both sides have the LTK, encryption and key distribution.
func (l *pairLink) pair(bonding bool) {
	l.t.Helper()
	if err := l.i.Pair(bonding, 0); err != nil {
		l.t.Fatal(err)
	}
	l.pump()
	if l.i.State() != StateDistributing || l.r.State() != StateDistributing && l.r.State() != StatePaired {
		return
	}
	var li, lr [16]byte
	if !l.i.LTK(&li) || !l.r.LTK(&lr) || li != lr {
		l.t.Fatalf("LTK initiator % x, responder % x", li, lr)
	}
	// Both controllers report encryption at the same time.
	if err := l.r.Encrypted(0); err != nil {
		l.t.Fatal(err)
	}
	if err := l.i.Encrypted(0); err != nil {
		l.t.Fatal(err)
	}
	l.pump()
}

func (l *pairLink) close() {
	l.t.Helper()
	l.i.Close()
	l.r.Close()
	l.imem.check(l.t)
	l.rmem.check(l.t)
}

var (
	initiatorIRK = [16]byte{0xc0, 0xc1, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, 0xc8, 0xc9, 0xca, 0xcb, 0xcc, 0xcd, 0xce, 0xcf}
	// initiatorID is a static random address: the top two bits of the last byte are 11.
	initiatorID = Address{Type: 1, Addr: [6]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0xd6}}
)

func TestInitiatorJustWorksWithKeys(t *testing.T) {
	l := newPairLink(t, IONoInputNoOutput, IONoInputNoOutput, nil, nil)
	l.i.SetIdentity(&initiatorIRK, initiatorID)
	l.r.SetIdentity(&ownIRK, ownIdentity)
	l.pair(true)
	var bi, br Bond
	if !l.i.Bond(&bi) || !l.r.Bond(&br) {
		t.Fatalf("states %d and %d, errors %v and %v", l.i.State(), l.r.State(), l.i.Err(), l.r.Err())
	}
	if bi.LTK != br.LTK || !bi.Bonded || !br.Bonded || bi.KeySize != 16 {
		t.Errorf("bonds %+v and %+v", bi, br)
	}
	if bi.PeerIRK != ownIRK || bi.PeerIdentity != ownIdentity || br.PeerIRK != initiatorIRK || br.PeerIdentity != initiatorID {
		t.Errorf("identities: initiator has %x %+v, responder has %x %+v", bi.PeerIRK, bi.PeerIdentity, br.PeerIRK, br.PeerIdentity)
	}
	l.close()
}

func TestInitiatorNoBonding(t *testing.T) {
	l := newPairLink(t, IONoInputNoOutput, IONoInputNoOutput, nil, nil)
	l.pair(false)
	var bi Bond
	if !l.i.Bond(&bi) || bi.Bonded || bi.HasPeerIRK {
		t.Errorf("state %d, bond %+v", l.i.State(), bi)
	}
	l.close()
}

func TestInitiatorNumericComparison(t *testing.T) {
	ci, cr := &recordConfirm{ok: true}, &recordConfirm{ok: true}
	l := newPairLink(t, IODisplayYesNo, IOKeyboardDisplay, ci, cr)
	l.pair(true)
	if l.i.State() != StatePaired || len(ci.values) != 1 || len(cr.values) != 1 || ci.values[0] != cr.values[0] {
		t.Errorf("state %d, shown %v and %v", l.i.State(), ci.values, cr.values)
	}
	l.close()

	no := &recordConfirm{}
	l = newPairLink(t, IODisplayYesNo, IODisplayYesNo, no, &recordConfirm{ok: true})
	l.pair(true)
	if l.i.State() != StateFailed || l.r.State() != StateFailed {
		t.Errorf("user says no: states %d and %d", l.i.State(), l.r.State())
	}
	l.close()
}

// sharedPasskey is the user: it shows the passkey of the responder and types it at the initiator.
type sharedPasskey struct {
	value uint32
	typed uint32
}

func (s *sharedPasskey) ShowPasskey(v uint32)         { s.value = v }
func (s *sharedPasskey) EnterPasskey() (uint32, bool) { return s.value + s.typed, true }

func TestInitiatorPasskey(t *testing.T) {
	// The responder shows the passkey and the user types it at the initiator.
	user := &sharedPasskey{}
	l := newPairLink(t, IOKeyboardOnly, IODisplayOnly, nil, nil)
	l.r.SetPasskeyIO(user, nil)
	l.i.SetPasskeyIO(nil, user)
	l.pair(true)
	if l.i.State() != StatePaired || l.r.State() != StatePaired {
		t.Fatalf("states %d and %d, errors %v and %v", l.i.State(), l.r.State(), l.i.Err(), l.r.Err())
	}
	l.close()

	// Both sides have only a keyboard, and the user types the same passkey on both.
	both := typed{passkey: 424242, ok: true}
	l = newPairLink(t, IOKeyboardOnly, IOKeyboardOnly, nil, nil)
	l.r.SetPasskeyIO(nil, both)
	l.i.SetPasskeyIO(nil, both)
	l.pair(true)
	if l.i.State() != StatePaired {
		t.Errorf("both type: state %d, error %v", l.i.State(), l.i.Err())
	}
	l.close()

	// A typing error in bit 1 fails in round 1 at the responder, and the initiator hears it.
	wrong := &sharedPasskey{typed: 2}
	l = newPairLink(t, IOKeyboardOnly, IODisplayOnly, nil, nil)
	l.r.SetPasskeyIO(wrong, nil)
	l.i.SetPasskeyIO(nil, wrong)
	l.pair(true)
	if l.i.State() != StateFailed || l.r.State() != StateFailed {
		t.Errorf("wrong passkey: states %d and %d", l.i.State(), l.r.State())
	}
	l.close()
}

// respond gives the initiator one Pairing Response and returns the Pairing Failed reason, or 0.
func respond(t *testing.T, pres []byte) byte {
	t.Helper()
	q := &queue{}
	mem := newCheckedMemory(t)
	i, _ := NewInitiator(mem, rand.New(rand.NewSource(1)), q, nil, IONoInputNoOutput)
	i.Start(central, peripheral)
	i.Pair(true, 0)
	q.pdus = nil
	if err := i.Handle(pres, 0); err != nil {
		t.Fatal(err)
	}
	var reason byte
	if len(q.pdus) > 0 && q.pdus[0][0] == codePairingFailed {
		reason = q.pdus[0][1]
	}
	i.Close()
	mem.check(t)
	return reason
}

func TestInitiatorChecksResponse(t *testing.T) {
	tests := []struct {
		name string
		pres []byte
		want byte
	}{
		{name: "accepted", pres: []byte{codePairingResponse, IONoInputNoOutput, 0, authSC, 16, 0, 0}, want: 0},
		{name: "legacy", pres: []byte{codePairingResponse, IONoInputNoOutput, 0, authBonding, 16, 0, 0}, want: ReasonAuthenticationRequirement},
		{name: "key size 6", pres: []byte{codePairingResponse, IONoInputNoOutput, 0, authSC, 6, 0, 0}, want: ReasonEncryptionKeySize},
		{name: "key not asked", pres: []byte{codePairingResponse, IONoInputNoOutput, 0, authSC, 16, 0x04, 0}, want: ReasonInvalidParameters},
		{name: "IO value 7", pres: []byte{codePairingResponse, 7, 0, authSC, 16, 0, 0}, want: ReasonPairingNotSupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := respond(t, tt.pres); got != tt.want {
				t.Errorf("reason %#x, want %#x", got, tt.want)
			}
		})
	}
}

func TestSecurityRequest(t *testing.T) {
	q := &queue{}
	i, _ := NewInitiator(newCheckedMemory(t), rand.New(rand.NewSource(1)), q, nil, IONoInputNoOutput)
	i.Start(central, peripheral)
	if err := i.Handle([]byte{codeSecurityRequest, authBonding | authSC}, 0); err != nil || !i.SecurityRequested() || len(q.pdus) != 0 {
		t.Errorf("security request: %v, recorded %v, sent % x", err, i.SecurityRequested(), q.pdus)
	}
	i.Start(central, peripheral)
	if i.SecurityRequested() {
		t.Error("Start does not clear the security request")
	}
}
