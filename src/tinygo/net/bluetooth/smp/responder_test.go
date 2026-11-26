package smp

import (
	"bytes"
	"crypto/aes"
	"crypto/ecdh"
	"encoding/binary"
	"math/rand"
	"testing"
)

// The initiator in these tests uses crypto/aes and crypto/ecdh, and not the toolbox of this package.

func refCMAC(key, msg []byte) []byte {
	c, _ := aes.NewCipher(key)
	var l, k1, k2 [16]byte
	c.Encrypt(l[:], l[:])
	dbl := func(dst, src []byte) {
		var carry byte
		for i := 15; i >= 0; i-- {
			dst[i] = src[i]<<1 | carry
			carry = src[i] >> 7
		}
		if src[0]&0x80 != 0 {
			dst[15] ^= 0x87
		}
	}
	dbl(k1[:], l[:])
	dbl(k2[:], k1[:])
	n := (len(msg) + 15) / 16
	last := make([]byte, 16)
	if n > 0 && len(msg)%16 == 0 {
		copy(last, msg[(n-1)*16:])
		for i := range last {
			last[i] ^= k1[i]
		}
	} else {
		if n == 0 {
			n = 1
		}
		rest := msg[(n-1)*16:]
		copy(last, rest)
		last[len(rest)] = 0x80
		for i := range last {
			last[i] ^= k2[i]
		}
	}
	x := make([]byte, 16)
	for i := 0; i < n-1; i++ {
		for j := 0; j < 16; j++ {
			x[j] ^= msg[i*16+j]
		}
		c.Encrypt(x, x)
	}
	for j := 0; j < 16; j++ {
		x[j] ^= last[j]
	}
	c.Encrypt(x, x)
	return x
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func rev(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func specAddr(a Address) []byte { return append([]byte{a.Type}, rev(a.Addr[:])...) }

type queue struct{ pdus [][]byte }

func (q *queue) SendSMP(pdu []byte) error {
	q.pdus = append(q.pdus, append([]byte(nil), pdu...))
	return nil
}

func (q *queue) pop(t *testing.T, code byte, size int) []byte {
	t.Helper()
	if len(q.pdus) == 0 {
		t.Fatalf("no PDU, want code %#x", code)
	}
	p := q.pdus[0]
	q.pdus = q.pdus[1:]
	if p[0] != code || len(p) != size {
		t.Fatalf("PDU % x, want code %#x and %d bytes", p, code, size)
	}
	return p
}

type answer struct {
	ok    bool
	shown uint32
	calls int
}

func (a *answer) Confirm(v uint32) bool {
	a.shown, a.calls = v, a.calls+1
	return a.ok
}

var (
	central    = Address{Type: 1, Addr: [6]byte{0xce, 0xbf, 0x37, 0x37, 0x12, 0x56}}
	peripheral = Address{Type: 0, Addr: [6]byte{0xa7, 0xa6, 0xa5, 0xa4, 0xa3, 0xa2}}
)

// initiator runs the central side of LE Secure Connections pairing against r.
type initiator struct {
	t        *testing.T
	r        *Responder
	q        *queue
	rng      *rand.Rand
	ioCap    uint8
	auth     uint8
	keySize  uint8
	dist     [2]byte // initiator and responder key distribution
	breakEa  bool
	reflect  bool   // send the public key of the responder back to it
	pkaRaw   []byte // 0x04 || X || Y
	shown    uint32
	ltk      []byte // spec order
	now      uint32
	lastFail []byte
}

func (in *initiator) handle(pdu []byte) {
	in.t.Helper()
	if err := in.r.Handle(pdu, in.now); err != nil {
		in.t.Fatal(err)
	}
}

// run returns false when the responder sent Pairing Failed, and keeps that PDU in lastFail.
func (in *initiator) run() bool {
	t := in.t
	preq := []byte{codePairingRequest, in.ioCap, 0, in.auth, in.keySize, in.dist[0], in.dist[1]}
	in.handle(preq)
	if in.failed() {
		return false
	}
	pres := in.q.pop(t, codePairingResponse, 7)
	curve := ecdh.P256()
	ka, err := ecdhKey(in.rng)
	if err != nil {
		t.Fatal(err)
	}
	if in.reflect {
		in.pkaRaw = cat([]byte{4}, in.r.s.pkbX[:], in.r.s.pkbY[:])
	}
	if in.pkaRaw == nil {
		in.pkaRaw = ka.PublicKey().Bytes()
	}
	pkax, pkay := in.pkaRaw[1:33], in.pkaRaw[33:]
	in.handle(cat([]byte{codePublicKey}, rev(pkax), rev(pkay)))
	if in.failed() {
		return false
	}
	pk := in.q.pop(t, codePublicKey, 65)
	pkbx, pkby := rev(pk[1:33]), rev(pk[33:65])
	cb := rev(in.q.pop(t, codePairingConfirm, 17)[1:])
	na := make([]byte, 16)
	in.rng.Read(na)
	in.handle(cat([]byte{codePairingRandom}, rev(na)))
	if in.failed() {
		return false
	}
	nb := rev(in.q.pop(t, codePairingRandom, 17)[1:])
	if !bytes.Equal(cb, refCMAC(nb, cat(pkbx, pkax, []byte{0}))) {
		t.Fatal("Cb does not match f4(PKbx, PKax, Nb, 0)")
	}
	in.shown = binary.BigEndian.Uint32(refCMAC(na, cat(pkax, pkbx, nb))[12:]) % 1000000

	peer, err := curve.NewPublicKey(cat([]byte{4}, pkbx, pkby))
	if err != nil {
		t.Fatal(err)
	}
	dhKey, _ := ka.ECDH(peer)
	salt := []byte{0x6c, 0x88, 0x83, 0x91, 0xaa, 0xf5, 0xa5, 0x38, 0x60, 0x37, 0x0b, 0xdb, 0x5a, 0x60, 0x83, 0xbe}
	tk := refCMAC(salt, dhKey)
	a, b := specAddr(central), specAddr(peripheral)
	keyID := []byte{0x62, 0x74, 0x6c, 0x65}
	macKey := refCMAC(tk, cat([]byte{0}, keyID, na, nb, a, b, []byte{1, 0}))
	in.ltk = refCMAC(tk, cat([]byte{1}, keyID, na, nb, a, b, []byte{1, 0}))
	r := make([]byte, 16)
	ea := refCMAC(macKey, cat(na, nb, r, []byte{preq[3], preq[2], preq[1]}, a, b))
	if in.breakEa {
		ea[0] ^= 1
	}
	in.handle(cat([]byte{codeDHKeyCheck}, rev(ea)))
	if in.failed() {
		return false
	}
	eb := rev(in.q.pop(t, codeDHKeyCheck, 17)[1:])
	if !bytes.Equal(eb, refCMAC(macKey, cat(nb, na, r, []byte{pres[3], pres[2], pres[1]}, b, a))) {
		t.Fatal("Eb does not match f6")
	}
	return true
}

func (in *initiator) failed() bool {
	if len(in.q.pdus) > 0 && in.q.pdus[0][0] == codePairingFailed {
		in.lastFail = in.q.pdus[0]
		in.q.pdus = in.q.pdus[1:]
		return true
	}
	return false
}

func newPair(t *testing.T, mem *checkedMemory, ioB uint8, confirm Confirmer, seed int64) *initiator {
	t.Helper()
	q := &queue{}
	r, err := NewResponder(mem, rand.New(rand.NewSource(seed+100)), q, confirm, ioB)
	if err != nil {
		t.Fatal(err)
	}
	r.Start(peripheral, central)
	return &initiator{t: t, r: r, q: q, rng: rand.New(rand.NewSource(seed)),
		ioCap: IONoInputNoOutput, auth: authBonding | authSC, keySize: 16}
}

func TestJustWorks(t *testing.T) {
	mem := newCheckedMemory(t)
	for seed := int64(0); seed < 5; seed++ {
		in := newPair(t, mem, IONoInputNoOutput, nil, seed)
		if !in.run() {
			t.Fatalf("seed %d: pairing failed with % x", seed, in.lastFail)
		}
		var ltk [16]byte
		if in.r.State() != StatePaired || !in.r.LTK(&ltk) || !bytes.Equal(ltk[:], rev(in.ltk)) {
			t.Fatalf("seed %d: state %d, LTK % x, want % x", seed, in.r.State(), ltk, rev(in.ltk))
		}
		if len(in.q.pdus) != 0 {
			t.Errorf("seed %d: unexpected PDUs % x", seed, in.q.pdus)
		}
		in.r.Close()
		if in.r.LTK(&ltk) {
			t.Error("LTK is still there after Close")
		}
	}
	mem.check(t)
}

func TestNumericComparison(t *testing.T) {
	mem := newCheckedMemory(t)
	a := &answer{ok: true}
	in := newPair(t, mem, IODisplayYesNo, a, 7)
	in.ioCap, in.auth = IOKeyboardDisplay, authBonding|authSC|authMITM
	if !in.run() {
		t.Fatalf("pairing failed with % x", in.lastFail)
	}
	if a.calls != 1 || a.shown != in.shown {
		t.Errorf("confirm calls %d, shown %06d, want %06d", a.calls, a.shown, in.shown)
	}
	in.r.Close()

	no := &answer{}
	in = newPair(t, mem, IODisplayYesNo, no, 8)
	in.ioCap = IODisplayYesNo
	if in.run() || in.lastFail[1] != ReasonNumericComparisonFailed || in.r.Err() != ErrFailed {
		t.Errorf("rejected comparison: fail % x, err %v", in.lastFail, in.r.Err())
	}
	in.r.Close()
	mem.check(t)
}

func TestKeySizeMask(t *testing.T) {
	mem := newCheckedMemory(t)
	in := newPair(t, mem, IONoInputNoOutput, nil, 3)
	in.keySize = 7
	if !in.run() {
		t.Fatal("pairing failed")
	}
	var ltk [16]byte
	in.r.LTK(&ltk)
	want := rev(in.ltk)
	clear(want[7:])
	if !bytes.Equal(ltk[:], want) {
		t.Errorf("LTK % x, want % x", ltk, want)
	}
	in.r.Close()
	mem.check(t)
}

func TestFailures(t *testing.T) {
	tests := []struct {
		name   string
		ioB    uint8
		setup  func(in *initiator)
		reason byte
	}{
		{name: "legacy pairing", ioB: IONoInputNoOutput, setup: func(in *initiator) { in.auth = authBonding }, reason: ReasonAuthenticationRequirement},
		{name: "passkey needed", ioB: IODisplayOnly, setup: func(in *initiator) { in.ioCap = IOKeyboardOnly }, reason: ReasonPairingNotSupported},
		{name: "key too short", ioB: IONoInputNoOutput, setup: func(in *initiator) { in.keySize = 6 }, reason: ReasonEncryptionKeySize},
		{name: "key not on curve", ioB: IONoInputNoOutput, setup: func(in *initiator) {
			in.pkaRaw = make([]byte, 65)
			in.pkaRaw[64] = 1
		}, reason: ReasonDHKeyCheckFailed},
		{name: "own key sent back", ioB: IONoInputNoOutput, setup: func(in *initiator) { in.reflect = true }, reason: ReasonDHKeyCheckFailed},
		{name: "wrong DHKey check", ioB: IONoInputNoOutput, setup: func(in *initiator) { in.breakEa = true }, reason: ReasonDHKeyCheckFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			in := newPair(t, mem, tt.ioB, nil, 11)
			tt.setup(in)
			if in.run() {
				t.Fatal("pairing succeeded")
			}
			if in.lastFail[1] != tt.reason || in.r.State() != StateFailed {
				t.Errorf("fail PDU % x, state %d, want reason %#x", in.lastFail, in.r.State(), tt.reason)
			}
			in.r.Close()
			mem.check(t)
		})
	}
}

func TestProtocolErrors(t *testing.T) {
	mem := newCheckedMemory(t)
	in := newPair(t, mem, IONoInputNoOutput, nil, 1)
	in.handle(cat([]byte{codePairingRandom}, make([]byte, 16)))
	if !in.failed() || in.lastFail[1] != ReasonUnspecified {
		t.Errorf("random before request: % x", in.lastFail)
	}
	in.handle([]byte{0x0b, 0x2d})
	if !in.failed() || in.lastFail[1] != ReasonCommandNotSupported {
		t.Errorf("unknown command: % x", in.lastFail)
	}

	// Timeout: the peer stops after the Pairing Request.
	in.r.Start(peripheral, central)
	in.handle([]byte{codePairingRequest, IONoInputNoOutput, 0, authSC, 16, 0, 0})
	in.q.pop(t, codePairingResponse, 7)
	in.r.Tick(timeoutMs - 1)
	if in.r.State() != StatePairing {
		t.Error("pairing ended before the timeout")
	}
	in.r.Tick(timeoutMs)
	if in.r.State() != StateFailed || in.r.Err() != ErrTimeout {
		t.Errorf("state %d, err %v after the timeout", in.r.State(), in.r.Err())
	}

	// The peer ends pairing with Pairing Failed.
	in.handle([]byte{codePairingRequest, IONoInputNoOutput, 0, authSC, 16, 0, 0})
	in.q.pop(t, codePairingResponse, 7)
	in.handle([]byte{codePairingFailed, ReasonUnspecified})
	if in.r.State() != StateFailed || len(in.q.pdus) != 0 {
		t.Errorf("state %d, PDUs % x after peer failure", in.r.State(), in.q.pdus)
	}
	in.r.Close()
	if err := in.r.Handle([]byte{codePairingRequest}, 0); err != ErrClosed {
		t.Errorf("Handle after Close: %v", err)
	}
	if _, err := NewResponder(nil, nil, nil, nil, 0); err != ErrNilDependency {
		t.Errorf("nil dependencies: %v", err)
	}
	mem.check(t)
}
