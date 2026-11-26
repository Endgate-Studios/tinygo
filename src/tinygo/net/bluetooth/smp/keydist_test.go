package smp

import (
	"bytes"
	"crypto/aes"
	"math/rand"
	"testing"
)

// Core spec v5.4 Vol 3 Part H Appendix D.7, ah random address hash function.
func TestAH(t *testing.T) {
	mem := newCheckedMemory(t)
	k := b16(t, "ec0234a3 57c8ad05 341010a6 0a397d9b")
	got, err := AH(mem, &k, 0x708194)
	if err != nil || got != 0x0dfbaa {
		t.Errorf("ah = %06x, %v, want 0dfbaa", got, err)
	}
	mem.check(t)
}

func TestRPA(t *testing.T) {
	mem := newCheckedMemory(t)
	rng := rand.New(rand.NewSource(5))
	var irk, other [16]byte
	rng.Read(irk[:])
	rng.Read(other[:])
	for i := 0; i < 50; i++ {
		var addr [6]byte
		if err := NewRPA(mem, &irk, rng, &addr); err != nil {
			t.Fatal(err)
		}
		if addr[5]>>6 != 0b01 {
			t.Fatalf("% x is not a resolvable private address", addr)
		}
		// The hash with crypto/aes: e(IRK, padding || prand), with IRK and prand most significant byte first.
		c, _ := aes.NewCipher(rev(irk[:]))
		var block [16]byte
		copy(block[13:], rev(addr[3:6]))
		c.Encrypt(block[:], block[:])
		if !bytes.Equal(addr[0:3], rev(block[13:16])) {
			t.Fatalf("hash of % x differs from crypto/aes", addr)
		}
		if !ResolveAddress(mem, &irk, &addr) {
			t.Fatalf("% x does not resolve with its IRK", addr)
		}
		if ResolveAddress(mem, &other, &addr) {
			t.Fatalf("% x resolves with another IRK", addr)
		}
		bad := addr
		bad[0] ^= 1
		if ResolveAddress(mem, &irk, &bad) {
			t.Fatal("an address with a changed hash resolves")
		}
		bad = addr
		bad[5] |= 0xc0
		if ResolveAddress(mem, &irk, &bad) {
			t.Fatal("a static random address resolves")
		}
	}
	var addr [6]byte
	if err := NewRPA(mem, &irk, bytes.NewReader(nil), &addr); err != ErrRandom {
		t.Errorf("empty random source: %v", err)
	}
	mem.check(t)
}

var (
	ownIRK      = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	ownIdentity = Address{Type: 1, Addr: [6]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0xc6}}
	peerIRK     = []byte{0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf}
	peerIDAddr  = []byte{codeIdentityAddr, 0, 1, 2, 3, 4, 5, 6} // public address
)

// pairForKeys pairs until the DHKey check, with identity keys asked in both directions.
func pairForKeys(t *testing.T, mem *checkedMemory, withIdentity bool, dist [2]byte) *initiator {
	t.Helper()
	in := newPair(t, mem, IONoInputNoOutput, nil, 21)
	if withIdentity {
		in.r.SetIdentity(&ownIRK, ownIdentity)
	}
	in.dist = dist
	if !in.run() {
		t.Fatalf("pairing failed with % x", in.lastFail)
	}
	return in
}

func TestKeyDistribution(t *testing.T) {
	mem := newCheckedMemory(t)
	in := pairForKeys(t, mem, true, [2]byte{distIDKey | 0x05, distIDKey | 0x05})
	var ltk [16]byte
	var bond Bond
	if in.r.State() != StateDistributing || !in.r.LTK(&ltk) || in.r.Bond(&bond) {
		t.Fatalf("after the DHKey check: state %d, LTK ready %v", in.r.State(), in.r.LTK(&ltk))
	}
	if err := in.r.Encrypted(1000); err != nil {
		t.Fatal(err)
	}
	info := in.q.pop(t, codeIdentityInfo, 17)
	addr := in.q.pop(t, codeIdentityAddr, 8)
	if !bytes.Equal(info[1:], ownIRK[:]) || addr[1] != 1 || !bytes.Equal(addr[2:], ownIdentity.Addr[:]) {
		t.Errorf("own keys % x, % x", info, addr)
	}
	in.handle(cat([]byte{codeIdentityInfo}, peerIRK))
	in.handle(peerIDAddr)
	if !in.r.Bond(&bond) || in.r.State() != StatePaired {
		t.Fatalf("state %d after key distribution", in.r.State())
	}
	if !bytes.Equal(bond.PeerIRK[:], peerIRK) || !bond.HasPeerIRK || bond.PeerIdentity.Type != 0 ||
		!bytes.Equal(bond.PeerIdentity.Addr[:], peerIDAddr[2:]) || bond.LTK != ltk || bond.KeySize != 16 {
		t.Errorf("bond %+v", bond)
	}
	in.r.Close()
	mem.check(t)
}

func TestKeyDistributionOneWay(t *testing.T) {
	mem := newCheckedMemory(t)
	// No identity is set, so the responder only receives.
	in := pairForKeys(t, mem, false, [2]byte{distIDKey, distIDKey})
	in.r.Encrypted(0)
	if len(in.q.pdus) != 0 {
		t.Errorf("responder with no identity sent % x", in.q.pdus)
	}
	in.handle(cat([]byte{codeIdentityInfo}, peerIRK))
	in.handle(peerIDAddr)
	var bond Bond
	if !in.r.Bond(&bond) || !bond.HasPeerIRK {
		t.Error("peer identity not stored")
	}
	in.r.Close()

	// The initiator asks only for the keys of the responder.
	in = pairForKeys(t, mem, true, [2]byte{0, distIDKey})
	in.r.Encrypted(0)
	in.q.pop(t, codeIdentityInfo, 17)
	in.q.pop(t, codeIdentityAddr, 8)
	if !in.r.Bond(&bond) || bond.HasPeerIRK {
		t.Errorf("state %d, has peer IRK %v", in.r.State(), bond.HasPeerIRK)
	}
	in.r.Close()
	mem.check(t)
}

func TestKeyDistributionFailures(t *testing.T) {
	tests := []struct {
		name   string
		act    func(in *initiator)
		reason byte
	}{
		{name: "keys before encryption", act: func(in *initiator) {
			in.handle(cat([]byte{codeIdentityInfo}, peerIRK))
		}, reason: ReasonUnspecified},
		{name: "address before IRK", act: func(in *initiator) {
			in.r.Encrypted(0)
			in.handle(peerIDAddr)
		}, reason: ReasonUnspecified},
		{name: "identity is a private address", act: func(in *initiator) {
			in.r.Encrypted(0)
			in.handle(cat([]byte{codeIdentityInfo}, peerIRK))
			in.handle([]byte{codeIdentityAddr, 1, 1, 2, 3, 4, 5, 0x46})
		}, reason: ReasonInvalidParameters},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			in := pairForKeys(t, mem, false, [2]byte{distIDKey, 0})
			tt.act(in)
			var ltk [16]byte
			if !in.failed() || in.lastFail[1] != tt.reason || in.r.State() != StateFailed || in.r.LTK(&ltk) {
				t.Errorf("fail % x, state %d, LTK kept %v", in.lastFail, in.r.State(), in.r.LTK(&ltk))
			}
			in.r.Close()
			mem.check(t)
		})
	}

	mem := newCheckedMemory(t)
	in := pairForKeys(t, mem, false, [2]byte{distIDKey, 0})
	in.r.Encrypted(5000)
	in.r.Tick(5000 + timeoutMs)
	var ltk [16]byte
	if in.r.State() != StateFailed || in.r.Err() != ErrTimeout || in.r.LTK(&ltk) {
		t.Errorf("timeout in key distribution: state %d, err %v", in.r.State(), in.r.Err())
	}
	in.r.Close()
	mem.check(t)
}
