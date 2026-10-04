package smp

import (
	"bytes"
	"crypto/ecdh"
	"math/rand"
	"testing"
)

type shown struct{ passkey uint32 }

func (d *shown) ShowPasskey(v uint32) { d.passkey = v }

type typed struct {
	passkey uint32
	ok      bool
}

func (in typed) EnterPasskey() (uint32, bool) { return in.passkey, in.ok }

// passkeyInitiator runs passkey entry from the central side with the passkey that pick returns
// after the public keys are exchanged. It returns the LTK in wire order, or the failure reason.
func passkeyInitiator(t *testing.T, r *Responder, q *queue, ioA uint8, pick func() uint32) (ltk []byte, reason byte) {
	t.Helper()
	rng := rand.New(rand.NewSource(17))
	preq := []byte{codePairingRequest, ioA, 0, authBonding | authMITM | authSC, 16, 0, 0}
	failed := func() byte {
		if len(q.pdus) > 0 && q.pdus[0][0] == codePairingFailed {
			return q.pdus[0][1]
		}
		return 0
	}
	if err := r.Handle(preq, 0); err != nil {
		t.Fatal(err)
	}
	if f := failed(); f != 0 {
		return nil, f
	}
	pres := q.pop(t, codePairingResponse, 7)
	if pres[3]&authMITM == 0 {
		t.Error("Pairing Response does not set MITM for passkey entry")
	}
	ka, _ := ecdhKey(rng)
	pka := ka.PublicKey().Bytes()
	pkax := pka[1:33]
	if err := r.Handle(cat([]byte{codePublicKey}, rev(pkax), rev(pka[33:])), 0); err != nil {
		t.Fatal(err)
	}
	pk := q.pop(t, codePublicKey, 65)
	if f := failed(); f != 0 {
		return nil, f
	}
	pkbx, pkby := rev(pk[1:33]), rev(pk[33:65])
	passkey := pick()
	na, nb := make([]byte, 16), []byte(nil)
	for i := 0; i < 20; i++ {
		z := []byte{0x80 | byte(passkey>>i&1)}
		rng.Read(na)
		if err := r.Handle(cat([]byte{codePairingConfirm}, rev(cmacRef(na, cat(pkax, pkbx, z)))), 0); err != nil {
			t.Fatal(err)
		}
		cb := rev(q.pop(t, codePairingConfirm, 17)[1:])
		if err := r.Handle(cat([]byte{codePairingRandom}, rev(na)), 0); err != nil {
			t.Fatal(err)
		}
		if f := failed(); f != 0 {
			return nil, f
		}
		nb = rev(q.pop(t, codePairingRandom, 17)[1:])
		if !bytes.Equal(cb, cmacRef(nb, cat(pkbx, pkax, z))) {
			t.Fatalf("round %d: Cb does not match", i)
		}
	}
	peer, _ := ecdh.P256().NewPublicKey(cat([]byte{4}, pkbx, pkby))
	dh, _ := ka.ECDH(peer)
	salt := []byte{0x6c, 0x88, 0x83, 0x91, 0xaa, 0xf5, 0xa5, 0x38, 0x60, 0x37, 0x0b, 0xdb, 0x5a, 0x60, 0x83, 0xbe}
	tk := cmacRef(salt, dh)
	a, b := specAddr(central), specAddr(peripheral)
	keyID := []byte{0x62, 0x74, 0x6c, 0x65}
	macKey := cmacRef(tk, cat([]byte{0}, keyID, na, nb, a, b, []byte{1, 0}))
	key := cmacRef(tk, cat([]byte{1}, keyID, na, nb, a, b, []byte{1, 0}))
	rv := make([]byte, 16)
	rv[12], rv[13], rv[14], rv[15] = byte(passkey>>24), byte(passkey>>16), byte(passkey>>8), byte(passkey)
	ea := cmacRef(macKey, cat(na, nb, rv, []byte{preq[3], preq[2], preq[1]}, a, b))
	if err := r.Handle(cat([]byte{codeDHKeyCheck}, rev(ea)), 0); err != nil {
		t.Fatal(err)
	}
	eb := rev(q.pop(t, codeDHKeyCheck, 17)[1:])
	if !bytes.Equal(eb, cmacRef(macKey, cat(nb, na, rv, []byte{pres[3], pres[2], pres[1]}, b, a))) {
		t.Fatal("Eb does not match")
	}
	return rev(key), 0
}

// cmacRef is AES-CMAC from crypto/aes, from responder_test.go.
func cmacRef(key, msg []byte) []byte { return refCMAC(key, msg) }

func newPasskeyResponder(t *testing.T, mem *checkedMemory, ioB uint8, d PasskeyDisplay, in PasskeyInput) (*Responder, *queue) {
	t.Helper()
	q := &queue{}
	r, err := NewResponder(mem, rand.New(rand.NewSource(23)), q, nil, ioB)
	if err != nil {
		t.Fatal(err)
	}
	r.SetPasskeyIO(d, in)
	r.Start(peripheral, central)
	return r, q
}

func TestPasskeyResponderShows(t *testing.T) {
	mem := newCheckedMemory(t)
	d := &shown{passkey: 1 << 30}
	r, q := newPasskeyResponder(t, mem, IODisplayOnly, d, nil)
	ltk, reason := passkeyInitiator(t, r, q, IOKeyboardOnly, func() uint32 { return d.passkey })
	var got [16]byte
	if reason != 0 || d.passkey > 999999 || !r.LTK(&got) || !bytes.Equal(got[:], ltk) {
		t.Fatalf("reason %#x, shown %d, LTK % x, want % x", reason, d.passkey, got, ltk)
	}
	r.Close()
	mem.check(t)
}

func TestPasskeyResponderEnters(t *testing.T) {
	mem := newCheckedMemory(t)
	r, q := newPasskeyResponder(t, mem, IOKeyboardOnly, nil, typed{passkey: 123456, ok: true})
	ltk, reason := passkeyInitiator(t, r, q, IODisplayOnly, func() uint32 { return 123456 })
	var got [16]byte
	if reason != 0 || !r.LTK(&got) || !bytes.Equal(got[:], ltk) {
		t.Fatalf("reason %#x, LTK % x, want % x", reason, got, ltk)
	}
	r.Close()
	mem.check(t)
}

func TestPasskeyFailures(t *testing.T) {
	tests := []struct {
		name   string
		in     PasskeyInput
		pick   uint32
		reason byte
	}{
		// 123457 differs from 123456 in bit 0, so round 0 fails.
		{name: "wrong passkey", in: typed{passkey: 123456, ok: true}, pick: 123457, reason: ReasonConfirmValueFailed},
		// 123456 + 2^19 differs only in the last bit, so the check fails in round 19.
		{name: "wrong last bit", in: typed{passkey: 123456, ok: true}, pick: 123456 + 1<<19, reason: ReasonConfirmValueFailed},
		{name: "user cancels", in: typed{}, reason: ReasonPasskeyEntryFailed},
		{name: "not a 6-digit passkey", in: typed{passkey: 1000000, ok: true}, reason: ReasonPasskeyEntryFailed},
		{name: "no input set", in: nil, reason: ReasonPairingNotSupported},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			r, q := newPasskeyResponder(t, mem, IOKeyboardOnly, nil, tt.in)
			_, reason := passkeyInitiator(t, r, q, IODisplayOnly, func() uint32 { return tt.pick })
			if reason != tt.reason || r.State() != StateFailed {
				t.Errorf("reason %#x, state %d, want reason %#x", reason, r.State(), tt.reason)
			}
			r.Close()
			mem.check(t)
		})
	}
}

func TestMethodTable(t *testing.T) {
	tests := []struct {
		a, b uint8
		want pairMethod
	}{
		{IOKeyboardDisplay, IOKeyboardOnly, passkeyEnter},
		{IOKeyboardOnly, IOKeyboardDisplay, passkeyShow},
		{IOKeyboardOnly, IOKeyboardOnly, passkeyEnter},
		{IODisplayYesNo, IOKeyboardDisplay, numericComparison},
		{IODisplayOnly, IODisplayYesNo, justWorks},
		{IONoInputNoOutput, IOKeyboardDisplay, justWorks},
	}
	for _, tt := range tests {
		if got, ok := method(tt.a, tt.b); !ok || got != tt.want {
			t.Errorf("method(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	if _, ok := method(5, 0); ok {
		t.Error("IO capability 5 is accepted")
	}
	var v uint32
	for i := 0; i < 200; i++ {
		if !randomPasskey(rand.New(rand.NewSource(int64(i))), &v) || v > 999999 {
			t.Fatalf("passkey %d from seed %d", v, i)
		}
	}
}
