package eapol

import (
	"bytes"
	stdaes "crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// refKDF is KDF-SHA256 of IEEE 802.11-2020 12.7.1.6.2 on the standard library.
func refKDF(key []byte, label string, data []byte, n int) []byte {
	var out []byte
	for i := uint16(1); len(out) < n; i++ {
		h := hmac.New(sha256.New, key)
		h.Write([]byte{byte(i), byte(i >> 8)})
		h.Write([]byte(label))
		h.Write(data)
		h.Write([]byte{byte(n * 8), byte(n * 8 >> 8)})
		out = h.Sum(out)
	}
	return out[:n]
}

// refCMAC is AES-CMAC of RFC 4493, written apart from tinygo/crypto/aes.
func refCMAC(key, msg []byte) []byte {
	b, _ := stdaes.NewCipher(key)
	dbl := func(in []byte) []byte {
		out := make([]byte, 16)
		for i := 0; i < 15; i++ {
			out[i] = in[i]<<1 | in[i+1]>>7
		}
		out[15] = in[15] << 1
		if in[0]&0x80 != 0 {
			out[15] ^= 0x87
		}
		return out
	}
	l := make([]byte, 16)
	b.Encrypt(l, l)
	k1 := dbl(l)
	k2 := dbl(k1)
	x := make([]byte, 16)
	for len(msg) > 16 {
		for i := range x {
			x[i] ^= msg[i]
		}
		b.Encrypt(x, x)
		msg = msg[16:]
	}
	last := make([]byte, 16)
	copy(last, msg)
	k := k1
	if len(msg) < 16 {
		last[len(msg)] = 0x80
		k = k2
	}
	for i := range x {
		x[i] ^= last[i] ^ k[i]
	}
	b.Encrypt(x, x)
	return x
}

func TestSAEFourWay(t *testing.T) {
	mem := newCheckedMemory(t)
	pmk := bytes.Repeat([]byte{0x5a}, 32)
	s, err := NewSAE(mem, &counterRandom{}, pmk)
	if err != nil {
		t.Fatal(err)
	}
	// The RSN element of the SAE AKM (00-0F-AC:8) with MFPC and MFPR.
	rsn := []byte{48, 20, 1, 0, 0, 0x0f, 0xac, 4, 1, 0, 0, 0x0f, 0xac, 4, 1, 0, 0, 0x0f, 0xac, 8, 0xc0, 0}
	if err := s.Start(apAddr, staAddr, rsn, rsn); err != nil {
		t.Fatal(err)
	}
	a := &authenticator{t: t, pmk: pmk, aa: apAddr, spa: staAddr, rsn: rsn, gtk: bytes.Repeat([]byte{0x6b}, 16), gtkID: 1}
	for i := range a.anonce {
		a.anonce[i] = byte(0x80 + i)
	}
	reply := make([]byte, 256)

	// A frame of key descriptor version 2 is not for SAE.
	if _, _, err := s.Handle(a.message1(), reply); err != ErrVersion {
		t.Fatalf("version 2 message 1: %v, want ErrVersion", err)
	}
	m1 := a.frame(infoVersionAKM|infoPairwise|infoAck, a.anonce[:], nil)
	n, _, err := s.Handle(m1, reply)
	if err != nil {
		t.Fatal(err)
	}
	m2 := reply[:n]
	if info := binary.BigEndian.Uint16(m2[offKeyInfo:]); info != infoVersionAKM|infoPairwise|infoMIC {
		t.Fatalf("message 2 key info %#x", info)
	}
	copy(a.snonce[:], m2[offNonce:offNonce+32])
	var d []byte
	d = append(d, sorted(a.aa[:], a.spa[:])...)
	d = append(d, sorted(a.anonce[:], a.snonce[:])...)
	a.ptk = refKDF(pmk, "Pairwise key expansion", d, 48)
	check := append([]byte(nil), m2...)
	clear(check[offMIC : offMIC+16])
	if !bytes.Equal(refCMAC(a.ptk[:16], check), m2[offMIC:offMIC+16]) {
		t.Fatal("message 2 MIC is not the AES-CMAC of the KCK")
	}

	// Message 3 with the RSN element, the GTK and the IGTK KDEs.
	igtk := bytes.Repeat([]byte{0x49}, 16)
	ipn := []byte{1, 2, 3, 4, 5, 6}
	kde := append([]byte{0xdd, 28, 0x00, 0x0f, 0xac, 9, 4, 0}, ipn...)
	kde = append(kde, igtk...)
	data := refWrap(a.ptk[16:32], pad(append(append(append([]byte(nil), rsn...), a.gtkKDE()...), kde...)))
	m3 := a.frame(infoVersionAKM|infoPairwise|infoInstall|infoAck|infoMIC|infoSecure|infoEncrypted, a.anonce[:], data)
	copy(m3[offMIC:], refCMAC(a.ptk[:16], m3))
	n, ev, err := s.Handle(m3, reply)
	if err != nil || ev != EventPairwise {
		t.Fatalf("message 3: event %d, %v", ev, err)
	}
	m4 := reply[:n]
	check = append([]byte(nil), m4...)
	clear(check[offMIC : offMIC+16])
	if !bytes.Equal(refCMAC(a.ptk[:16], check), m4[offMIC:offMIC+16]) {
		t.Fatal("message 4 MIC is wrong")
	}
	if !bytes.Equal(s.TK(), a.ptk[32:48]) {
		t.Errorf("TK %x, want %x", s.TK(), a.ptk[32:48])
	}
	if g, id := s.GTK(); !bytes.Equal(g, a.gtk) || id != 1 {
		t.Errorf("GTK %x id %d", g, id)
	}
	gotIGTK, id, gotIPN := s.IGTK()
	if !bytes.Equal(gotIGTK, igtk) || id != 4 || !bytes.Equal(gotIPN, ipn) {
		t.Errorf("IGTK %x id %d IPN %x", gotIGTK, id, gotIPN)
	}
	s.Close()
	if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
	}
}
