package eapol

import (
	"encoding/binary"
	"testing"
)

// Fuzz tests of the two sides of the 4-way handshake. They check that no
// frame makes a side panic or write past its reply buffer, and that Close
// frees all memory after it zeroes it.

// fuzzPMK is the PMK of "password" and "IEEE", derived once.
var fuzzPMK []byte

func pmkOnce() []byte {
	if fuzzPMK == nil {
		fuzzPMK = pbkdf2([]byte("password"), []byte("IEEE"), 4096, 32)
	}
	return fuzzPMK
}

func FuzzKeyFrame(f *testing.F) {
	a := &authenticator{aa: apAddr, spa: staAddr}
	f.Add(a.frame(infoVersionAES|infoPairwise|infoAck, make([]byte, 32), nil))
	f.Fuzz(func(t *testing.T, b []byte) {
		parseKeyFrame(b)
	})
}

// FuzzSupplicant gives the supplicant a series of frames. Each step is a
// kind, a key info, a length and data: 0 sends a correct message 1 and
// derives the PTK from the answer, 1 a signed frame with the info and the
// data, 2 a signed frame with the data wrapped as key data, 3 the raw data.
func FuzzSupplicant(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{0, 0, 0, 0, 2, 0x13, 0xca, 0})
	f.Add([]byte{0, 0, 0, 0, 2, 0x13, 0xca, 0, 2, 0x03, 0x92, 0})
	f.Add([]byte{3, 0, 0, 4, 2, 3, 0, 0x5f})
	f.Fuzz(func(t *testing.T, steps []byte) {
		if len(steps) > 1024 {
			return
		}
		mem := newCheckedMemory(t)
		s, err := NewPSK(mem, &counterRandom{}, pmkOnce())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Start(apAddr, staAddr, rsnElem, rsnElem); err != nil {
			t.Fatal(err)
		}
		a := &authenticator{t: t, pmk: pmkOnce(), aa: apAddr, spa: staAddr, rsn: rsnElem, gtk: make([]byte, 16), gtkID: 1}
		reply := make([]byte, 256)
		for len(steps) >= 4 {
			kind, info, n := steps[0]&3, binary.BigEndian.Uint16(steps[1:]), int(steps[3])
			steps = steps[4:]
			n = min(n, len(steps))
			data := steps[:n]
			steps = steps[n:]
			var frame []byte
			switch kind {
			case 0:
				frame = a.message1()
			case 1, 2:
				if kind == 2 && a.ptk != nil {
					data = refWrap(a.ptk[16:32], pad(append([]byte(nil), data...)))
				}
				frame = a.frame(info, a.anonce[:], data)
				if a.ptk != nil {
					a.sign(frame)
				}
			case 3:
				frame = data
			}
			m, _, _ := s.Handle(frame, reply)
			if m > len(reply) {
				t.Fatalf("reply of %d bytes", m)
			}
			if kind == 0 && m >= offData {
				copy(a.snonce[:], reply[offNonce:offNonce+32])
				var d []byte
				d = append(d, sorted(a.aa[:], a.spa[:])...)
				d = append(d, sorted(a.anonce[:], a.snonce[:])...)
				a.ptk = refPRF(a.pmk, "Pairwise key expansion", d, 48)
			}
		}
		s.Close()
		if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
			t.Fatalf("after Close: in use %d, freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
		}
	})
}

// FuzzAuthenticator gives the access point side a fuzzed RSN element of the
// station and a series of frames. Each step is a kind and a position: 0 sends
// the answer of a real supplicant, 1 the same with one byte changed, 2 the
// raw step data, 3 a resend.
func FuzzAuthenticator(f *testing.F) {
	f.Add(rsnElem, []byte{0, 0, 0, 0})
	f.Add(rsnElem, []byte{1, 40, 0, 0, 0, 0})
	f.Add([]byte{48, 2, 1, 0}, []byte{2, 5, 1, 2, 3, 4, 5})
	f.Fuzz(func(t *testing.T, staRSN, steps []byte) {
		if len(steps) > 1024 || len(staRSN) > 255 {
			return
		}
		mem := newCheckedMemory(t)
		auth, err := NewAuthenticatorPMK(mem, &counterRandom{n: 100}, apAddr, rsnElem, pmkOnce())
		if err != nil {
			t.Fatal(err)
		}
		h, err := auth.NewHandshake(staAddr, staRSN)
		if err != nil {
			auth.Close()
			return
		}
		s, err := NewPSK(mem, &counterRandom{}, pmkOnce())
		if err != nil {
			t.Fatal(err)
		}
		s.Start(apAddr, staAddr, rsnElem, rsnElem)
		toSta, toAP := make([]byte, 256), make([]byte, 256)
		n, _ := h.Start(toSta)
		for len(steps) >= 2 {
			kind, pos := steps[0]&3, int(steps[1])
			steps = steps[2:]
			var frame []byte
			switch kind {
			case 0, 1:
				if n == 0 {
					continue
				}
				m, _, _ := s.Handle(toSta[:n], toAP)
				if m == 0 {
					continue
				}
				frame = toAP[:m]
				if kind == 1 {
					frame[pos%m] ^= 0x5a
				}
			case 2:
				k := min(pos, len(steps))
				frame, steps = steps[:k], steps[k:]
			case 3:
				n, _ = h.Resend(toSta)
				continue
			}
			m, _, _ := h.Handle(frame, toSta)
			if m > len(toSta) {
				t.Fatalf("reply of %d bytes", m)
			}
			if m > 0 {
				n = m
			}
		}
		h.Close()
		s.Close()
		auth.Close()
		if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
			t.Fatalf("after Close: in use %d, freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
		}
	})
}
