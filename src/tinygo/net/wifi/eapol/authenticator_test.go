package eapol

import (
	"bytes"
	"testing"
)

func newTestAuthenticator(t *testing.T, mem Memory) *Authenticator {
	t.Helper()
	a, err := NewAuthenticator(mem, &counterRandom{n: 100}, apAddr, rsnElem, []byte("password"), []byte("IEEE"))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestAuthenticatorWithSupplicant runs the 4-way handshake between the two
// sides of this package.
func TestAuthenticatorWithSupplicant(t *testing.T) {
	mem := newCheckedMemory(t)
	a := newTestAuthenticator(t, mem)
	h, err := a.NewHandshake(staAddr, rsnElem)
	if err != nil {
		t.Fatal(err)
	}
	s := newSupplicant(t, mem)
	toSta, toAP := make([]byte, 256), make([]byte, 256)

	n, err := h.Start(toSta)
	if err != nil || !h.Waiting() {
		t.Fatalf("message 1: %v", err)
	}
	m, ev, err := s.Handle(toSta[:n], toAP)
	if err != nil || ev != EventNone {
		t.Fatalf("supplicant message 1: %v", err)
	}
	n, done, err := h.Handle(toAP[:m], toSta)
	if err != nil || done || n == 0 {
		t.Fatalf("message 2: n %d, done %v, %v", n, done, err)
	}
	m, ev, err = s.Handle(toSta[:n], toAP)
	if err != nil || ev != EventPairwise {
		t.Fatalf("supplicant message 3: event %d, %v", ev, err)
	}
	n, done, err = h.Handle(toAP[:m], toSta)
	if err != nil || !done || n != 0 || h.Waiting() {
		t.Fatalf("message 4: n %d, done %v, %v", n, done, err)
	}
	if h.TK() == nil || !bytes.Equal(h.TK(), s.TK()) {
		t.Errorf("TK of the authenticator %x, of the supplicant %x", h.TK(), s.TK())
	}
	gtk, id := a.GTK()
	if sg, sid := s.GTK(); !bytes.Equal(sg, gtk) || sid != id || id != 1 {
		t.Errorf("GTK of the supplicant %x id %d, want %x id %d", sg, sid, gtk, id)
	}
	h.Close()
	s.Close()
	a.Close()
	if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
	}
}

func TestAuthenticatorWrongPassword(t *testing.T) {
	mem := newCheckedMemory(t)
	a := newTestAuthenticator(t, mem)
	h, _ := a.NewHandshake(staAddr, rsnElem)
	s, err := New(mem, &counterRandom{}, []byte("not the password"), []byte("IEEE"))
	if err != nil {
		t.Fatal(err)
	}
	s.Start(apAddr, staAddr, rsnElem, rsnElem)
	toSta, toAP := make([]byte, 256), make([]byte, 256)
	n, _ := h.Start(toSta)
	m, _, _ := s.Handle(toSta[:n], toAP)
	if _, _, err := h.Handle(toAP[:m], toSta); err != ErrMIC {
		t.Errorf("message 2 with a wrong password: %v, want ErrMIC", err)
	}
	if !h.Waiting() {
		t.Error("a bad message 2 ended the handshake")
	}
}

func TestAuthenticatorRSNMismatch(t *testing.T) {
	mem := newCheckedMemory(t)
	a := newTestAuthenticator(t, mem)
	other := bytes.Clone(rsnElem)
	other[len(other)-2] = 0 // other RSN capabilities
	h, err := a.NewHandshake(staAddr, other)
	if err != nil {
		t.Fatal(err)
	}
	s := newSupplicant(t, mem) // sends rsnElem in message 2
	toSta, toAP := make([]byte, 256), make([]byte, 256)
	n, _ := h.Start(toSta)
	m, _, _ := s.Handle(toSta[:n], toAP)
	if _, _, err := h.Handle(toAP[:m], toSta); err != ErrRSNMismatch {
		t.Errorf("message 2 with another RSN element: %v, want ErrRSNMismatch", err)
	}
}

func TestAuthenticatorResend(t *testing.T) {
	mem := newCheckedMemory(t)
	a := newTestAuthenticator(t, mem)
	h, _ := a.NewHandshake(staAddr, rsnElem)
	toSta := make([]byte, 256)
	if _, err := h.Start(toSta); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxResends; i++ {
		n, err := h.Resend(toSta)
		if err != nil || n == 0 {
			t.Fatalf("resend %d: %v", i, err)
		}
		f, _ := parseKeyFrame(toSta[:n], infoVersionAES)
		if f.replay != uint64(i+2) {
			t.Errorf("resend %d replay counter %d, want %d", i, f.replay, i+2)
		}
	}
	if _, err := h.Resend(toSta); err != ErrTimeout {
		t.Errorf("resend after %d: %v, want ErrTimeout", MaxResends, err)
	}

	// The supplicant answers the last resend.
	h2, _ := a.NewHandshake(staAddr, rsnElem)
	s := newSupplicant(t, mem)
	toAP := make([]byte, 256)
	h2.Start(toSta)
	n, _ := h2.Resend(toSta)
	m, _, err := s.Handle(toSta[:n], toAP)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h2.Handle(toAP[:m], toSta); err != nil {
		t.Errorf("message 2 after a resend: %v", err)
	}
}

func TestAuthenticatorStationRSN(t *testing.T) {
	mem := newCheckedMemory(t)
	a := newTestAuthenticator(t, mem)
	tkip := bytes.Clone(rsnElem)
	tkip[13] = 2 // the pairwise cipher is TKIP
	if _, err := a.NewHandshake(staAddr, tkip); err != ErrStationRSN {
		t.Errorf("TKIP station: %v, want ErrStationRSN", err)
	}
	if _, err := a.NewHandshake(staAddr, []byte{48, 2, 1, 0}); err != ErrStationRSN {
		t.Errorf("short RSN element: %v, want ErrStationRSN", err)
	}
}

// TestAuthenticatorPMK uses the PMK of IEEE 802.11-2020 J.4.2 for "password"
// and "IEEE".
func TestAuthenticatorPMK(t *testing.T) {
	pmk := []byte{
		0xf4, 0x2c, 0x6f, 0xc5, 0x2d, 0xf0, 0xeb, 0xef, 0x9e, 0xbb, 0x4b, 0x90, 0xb3, 0x8a, 0x5f, 0x90,
		0x2e, 0x83, 0xfe, 0x1b, 0x13, 0x5a, 0x70, 0xe2, 0x3a, 0xed, 0x76, 0x2e, 0x97, 0x10, 0xa1, 0x2e,
	}
	mem := newCheckedMemory(t)
	a := newTestAuthenticator(t, mem)
	b, err := NewAuthenticatorPMK(mem, &counterRandom{n: 100}, apAddr, rsnElem, pmk)
	if err != nil {
		t.Fatal(err)
	}
	if a.pmk != b.pmk {
		t.Errorf("PMK %x, want %x", a.pmk, b.pmk)
	}
	a.Close()
	b.Close()
	if _, err := NewAuthenticatorPMK(mem, &counterRandom{n: 100}, apAddr, rsnElem, pmk[:16]); err == nil {
		t.Error("a short PMK gave no error")
	}
}
