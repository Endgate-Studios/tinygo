package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	stdsha256 "crypto/sha256"
	"io"
	"testing"
	"time"

	"runtime/memory/arena"
	"tinygo/crypto/hkdf"
	"tinygo/crypto/x25519"
)

// scriptServer is a TLS 1.3 server with a fixed flight: ServerHello, then
// EncryptedExtensions with ee, the chain of ch, CertificateVerify and
// Finished, then application records with the plaintext sizes of sizes.
type scriptServer struct {
	ch    *chain
	ee    []byte
	sizes []int
	sent  []byte
	out   []byte
	built bool
	err   error
}

func (s *scriptServer) Write(b []byte) (int, error) {
	s.sent = append(s.sent, b...)
	return len(b), nil
}

func (s *scriptServer) SetReadDeadline(time.Time) {}

func (s *scriptServer) Read(b []byte) (int, error) {
	if !s.built {
		s.built = true
		s.err = s.build()
	}
	if s.err != nil {
		return 0, s.err
	}
	if len(s.out) == 0 {
		return 0, io.EOF
	}
	n := copy(b, s.out)
	s.out = s.out[n:]
	return n, nil
}

func (s *scriptServer) build() error {
	mem, _ := arena.NewHeap(make([]byte, 8<<10))
	ch := s.sent[headerLen : headerLen+(int(s.sent[3])<<8|int(s.sent[4]))]
	sid := ch[4+2+32+1 : 4+2+32+1+32]
	i := bytes.Index(ch, []byte{0, 0x33, 0, 38, 0, 36, 0, 0x1d, 0, 32})
	var clientPub, serverPub, shared [32]byte
	copy(clientPub[:], ch[i+10:])
	x25519.X25519(&serverPub, &fuzzServerPriv, &x25519.Basepoint)
	x25519.X25519(&shared, &fuzzServerPriv, &clientPub)

	sh := []byte{3, 3}
	sh = append(sh, make([]byte, 32)...)
	sh = append(sh, 32)
	sh = append(sh, sid...)
	sh = append(sh, suiteAES128GCMSHA256>>8, suiteAES128GCMSHA256&0xff, 0)
	sh = append(sh, 0, 46, 0, 0x2b, 0, 2, 3, 4, 0, 0x33, 0, 36, 0, 0x1d, 0, 32)
	sh = append(sh, serverPub[:]...)
	shMsg := handshakeMsg(hsServerHello, sh)
	s.out = append([]byte{recordHandshake, 3, 3, byte(len(shMsg) >> 8), byte(len(shMsg))}, shMsg...)

	transcript := stdsha256.New()
	transcript.Write(ch)
	transcript.Write(shMsg)
	th := func() []byte { return transcript.Sum(nil) }
	var early, derived, hs, sHS, zero [32]byte
	hkdf.Extract(&early, nil, zero[:])
	deriveSecret(&derived, early[:], "derived", emptyHash[:])
	hkdf.Extract(&hs, derived[:], shared[:])
	deriveSecret(&sHS, hs[:], "s hs traffic", th())

	var flight []byte
	add := func(typ byte, body []byte) {
		m := handshakeMsg(typ, body)
		transcript.Write(m)
		flight = append(flight, chunk(recordHandshake, m)...)
	}
	add(hsEncryptedExtensions, append([]byte{byte(len(s.ee) >> 8), byte(len(s.ee))}, s.ee...))
	var list []byte
	for _, der := range s.ch.serverCert.Certificate {
		n := len(der)
		list = append(list, byte(n>>16), byte(n>>8), byte(n))
		list = append(list, der...)
		list = append(list, 0, 0)
	}
	add(hsCertificate, append([]byte{0, byte(len(list) >> 16), byte(len(list) >> 8), byte(len(list))}, list...))
	msg := append(bytes.Repeat([]byte{0x20}, 64), "TLS 1.3, server CertificateVerify\x00"...)
	digest := stdsha256.Sum256(append(msg, th()...))
	sig, err := ecdsa.SignASN1(rand.Reader, s.ch.serverCert.PrivateKey.(*ecdsa.PrivateKey), digest[:])
	if err != nil {
		return err
	}
	add(hsCertificateVerify, append([]byte{4, 3, byte(len(sig) >> 8), byte(len(sig))}, sig...))
	var fin [32]byte
	finished(&fin, sHS[:], th())
	add(hsFinished, fin[:])

	var k trafficKeys
	if err := k.set(mem, sHS[:]); err != nil {
		return err
	}
	s.out = append(s.out, sealRecords(&k, flight)...)
	k.close()

	// The application records, with the keys after the server Finished.
	var master, sAP [32]byte
	deriveSecret(&derived, hs[:], "derived", emptyHash[:])
	hkdf.Extract(&master, derived[:], zero[:])
	deriveSecret(&sAP, master[:], "s ap traffic", th())
	if err := k.set(mem, sAP[:]); err != nil {
		return err
	}
	var app []byte
	for _, n := range s.sizes {
		app = append(app, chunk(recordData, bytes.Repeat([]byte{'a'}, n))...)
	}
	s.out = append(s.out, sealRecords(&k, app)...)
	k.close()
	return nil
}

// TestRecordLimit checks that the client takes a smaller receive buffer
// when the server agrees to record_size_limit or max_fragment_length, that
// records up to the limit pass, and that a larger record fails.
func TestRecordLimit(t *testing.T) {
	ch := newChain(t, "node.example")
	for _, tc := range []struct {
		name  string
		ask   int
		ee    []byte
		limit int
	}{
		{"record_size_limit", 1000, []byte{0, 0x1c, 0, 2, 0x40, 0x01}, 1000},
		{"max_fragment_length", 1000, []byte{0, 0x01, 0, 1, 1}, 512},
		{"no agreement", 1000, nil, 0},
	} {
		srv := &scriptServer{ch: ch, ee: tc.ee}
		if tc.limit > 0 {
			srv.sizes = []int{tc.limit, 1, tc.limit + 1}
		} else {
			srv.sizes = []int{4000} // sealRecords takes at most 4095 bytes
		}
		mem, _ := arena.NewHeap(make([]byte, 48<<10))
		c := Client(srv, mem, rand.Reader, &Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}, RecordSizeLimit: tc.ask})
		if err := c.Handshake(); err != nil {
			t.Fatalf("%s: handshake: %v", tc.name, err)
		}
		if c.RecordLimit() != tc.limit {
			t.Errorf("%s: limit %d, want %d", tc.name, c.RecordLimit(), tc.limit)
		}
		if tc.limit > 0 && len(c.rx) != headerLen+tc.limit+256 {
			t.Errorf("%s: receive buffer %d bytes", tc.name, len(c.rx))
		}
		buf := make([]byte, maxPlain)
		got := 0
		var err error
		for err == nil {
			var n int
			n, err = c.Read(buf)
			got += n
		}
		if tc.limit > 0 && (got != tc.limit+1 || err != ErrRecord) {
			t.Errorf("%s: read %d bytes, then %v", tc.name, got, err)
		}
		if tc.limit == 0 && got != 4000 {
			t.Errorf("%s: read %d bytes, then %v", tc.name, got, err)
		}
		c.Close()
		if mem.Stats().InUse != 0 {
			t.Errorf("%s: memory in use after Close: %d", tc.name, mem.Stats().InUse)
		}
	}
	// The server may only agree to what the client asked for.
	srv := &scriptServer{ch: ch, ee: []byte{0, 0x01, 0, 1, 2}}
	mem, _ := arena.NewHeap(make([]byte, 48<<10))
	c := Client(srv, mem, rand.Reader, &Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}, RecordSizeLimit: 600})
	if err := c.Handshake(); err != ErrUnsupported {
		t.Errorf("other max_fragment_length: %v, want %v", err, ErrUnsupported)
	}
}
