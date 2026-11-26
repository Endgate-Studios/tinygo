package tls

import (
	"bytes"
	"io"
	"testing"
	"time"

	"runtime/memory/arena"
	"tinygo/crypto/hkdf"
	"tinygo/crypto/sha256"
	"tinygo/crypto/x25519"
)

// countReader is a fixed random source, so a fuzz input always gives the
// same run.
type countReader struct{ n byte }

func (r *countReader) Read(b []byte) (int, error) {
	for i := range b {
		r.n++
		b[i] = r.n
	}
	return len(b), nil
}

// fuzzServer is the transport of FuzzClient. In mode 0 the fuzz bytes are the
// server stream. In mode 1 the first Read answers the ClientHello with a
// valid ServerHello, then sends the fuzz bytes in records with the server
// handshake keys. In mode 2 the client has finished the handshake, and the
// fuzz bytes go in records with the server application keys.
type fuzzServer struct {
	data  []byte
	mode  byte
	keys  *trafficKeys // the server write keys of mode 2
	mem   Memory
	sent  []byte
	out   []byte
	built bool
}

func (f *fuzzServer) Write(b []byte) (int, error) {
	f.sent = append(f.sent, b...)
	return len(b), nil
}

func (f *fuzzServer) SetReadDeadline(time.Time) {}

func (f *fuzzServer) Read(b []byte) (int, error) {
	if !f.built {
		f.built = true
		f.build()
	}
	if len(f.out) == 0 {
		return 0, io.EOF
	}
	n := copy(b, f.out)
	f.out = f.out[n:]
	return n, nil
}

var fuzzServerPriv = [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9}

func (f *fuzzServer) build() {
	switch f.mode {
	case 0:
		f.out = f.data
		return
	case 2:
		f.out = sealRecords(f.keys, f.data)
		return
	}
	// The ClientHello is the first record. Its session ID is at a fixed
	// place, and its only key share is X25519.
	if len(f.sent) < headerLen+4+2+32+1+32 {
		return
	}
	ch := f.sent[headerLen : headerLen+(int(f.sent[3])<<8|int(f.sent[4]))]
	sid := ch[4+2+32+1 : 4+2+32+1+32]
	i := bytes.Index(ch, []byte{0, 0x33, 0, 38, 0, 36, 0, 0x1d, 0, 32})
	if i < 0 || i+10+32 > len(ch) {
		return
	}
	var clientPub, serverPub, shared [32]byte
	copy(clientPub[:], ch[i+10:])
	x25519.X25519(&serverPub, &fuzzServerPriv, &x25519.Basepoint)
	x25519.X25519(&shared, &fuzzServerPriv, &clientPub)

	sh := []byte{hsServerHello, 0, 0, 0, 3, 3}
	sh = append(sh, make([]byte, 32)...)
	sh = append(sh, 32)
	sh = append(sh, sid...)
	sh = append(sh, suiteAES128GCMSHA256>>8, suiteAES128GCMSHA256&0xff, 0)
	sh = append(sh, 0, 46, 0, 0x2b, 0, 2, 3, 4, 0, 0x33, 0, 36, 0, 0x1d, 0, 32)
	sh = append(sh, serverPub[:]...)
	sh[3] = byte(len(sh) - 4)
	f.out = append([]byte{recordHandshake, 3, 3, 0, byte(len(sh))}, sh...)
	f.out = append(f.out, recordCCS, 3, 3, 0, 1, 1)

	// The server handshake keys, as the client derives them.
	var d sha256.Digest
	d.Reset(sha256.Init, 0)
	d.Write(ch)
	d.Write(sh)
	var th, early, derived, hs, sHS, zero [32]byte
	d.Sum(&th)
	hkdf.Extract(&early, nil, zero[:])
	deriveSecret(&derived, early[:], "derived", emptyHash[:])
	hkdf.Extract(&hs, derived[:], shared[:])
	deriveSecret(&sHS, hs[:], "s hs traffic", th[:])
	var k trafficKeys
	if k.set(f.mem, sHS[:]) != nil {
		return
	}
	f.out = append(f.out, sealRecords(&k, f.data)...)
	k.close()
}

// sealRecords encrypts the chunks of data into records. Each chunk has a
// 2-byte length, and its last byte is the inner content type.
func sealRecords(k *trafficKeys, data []byte) []byte {
	var out []byte
	for len(data) >= 2 {
		n := (int(data[0])<<8 | int(data[1])) % 4096
		data = data[2:]
		n = min(n, len(data))
		rec := make([]byte, headerLen+n+tagLen)
		rec[0], rec[1], rec[2] = recordData, 3, 3
		rec[3], rec[4] = byte((n+tagLen)>>8), byte(n+tagLen)
		nonce := k.nonce()
		k.gcm.Seal(rec[headerLen:], &nonce, data[:n], rec[:headerLen])
		k.seq++
		out = append(out, rec...)
		data = data[n:]
	}
	return out
}

// chunk gives a chunk of sealRecords: msg followed by the inner type.
func chunk(typ byte, msg []byte) []byte {
	n := len(msg) + 1
	return append(append([]byte{byte(n >> 8), byte(n)}, msg...), typ)
}

func handshakeMsg(typ byte, body []byte) []byte {
	n := len(body)
	return append([]byte{typ, byte(n >> 16), byte(n >> 8), byte(n)}, body...)
}

// fuzzSeeds gives one input for each mode.
func fuzzSeeds(ch *chain) [][]byte {
	var seeds [][]byte
	// Mode 1: EncryptedExtensions, the chain, a bad CertificateVerify and a
	// bad Finished.
	var list []byte
	for _, der := range ch.serverCert.Certificate {
		n := len(der)
		list = append(list, byte(n>>16), byte(n>>8), byte(n))
		list = append(list, der...)
		list = append(list, 0, 0)
	}
	cert := append([]byte{0, byte(len(list) >> 16), byte(len(list) >> 8), byte(len(list))}, list...)
	seed := []byte{1}
	seed = append(seed, chunk(recordHandshake, handshakeMsg(hsEncryptedExtensions, []byte{0, 0}))...)
	seed = append(seed, chunk(recordHandshake, handshakeMsg(hsCertificate, cert))...)
	seed = append(seed, chunk(recordHandshake, handshakeMsg(hsCertificateVerify, []byte{4, 3, 0, 2, 0, 0}))...)
	seed = append(seed, chunk(recordHandshake, handshakeMsg(hsFinished, make([]byte, 32)))...)
	seeds = append(seeds, seed)
	// Mode 0: a HelloRetryRequest and an alert.
	hrr := []byte{3, 3}
	hrr = append(hrr, helloRetry[:]...)
	hrr = append(hrr, 32)
	hrr = append(hrr, make([]byte, 32)...)
	hrr = append(hrr, 0x13, 0x01, 0, 0, 12, 0, 0x2b, 0, 2, 3, 4, 0, 0x33, 0, 2, 0, 0x17)
	msg := handshakeMsg(hsServerHello, hrr)
	seeds = append(seeds, append(append([]byte{0, recordHandshake, 3, 3, 0, byte(len(msg))}, msg...), recordAlert, 3, 3, 0, 2, 2, 40))
	// Mode 2: data, a ticket, a KeyUpdate and close_notify.
	seed = []byte{2}
	seed = append(seed, chunk(recordData, []byte("hello"))...)
	seed = append(seed, chunk(recordHandshake, handshakeMsg(hsNewSessionTicket, make([]byte, 13)))...)
	seed = append(seed, chunk(recordHandshake, handshakeMsg(hsKeyUpdate, []byte{1}))...)
	seed = append(seed, chunk(recordAlert, []byte{1, 0})...)
	return append(seeds, seed)
}

// runFuzz runs the client on one input and gives the first error and the
// bytes that Read gave.
func runFuzz(t *testing.T, ch *chain, heap, data []byte) (error, []byte) {
	if len(data) == 0 {
		return nil, nil
	}
	mem, _ := arena.NewHeap(heap)
	srv := &fuzzServer{data: data[1:], mode: data[0] % 3, mem: mem}
	c := Client(srv, mem, &countReader{}, &Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}, Session: &Session{}, RecordSizeLimit: 1024})
	buf := make([]byte, 256)
	var first error
	var got []byte
	if srv.mode == 2 {
		// A finished handshake with known application secrets.
		c.rx = make([]byte, headerLen+maxRecord)
		c.tx = make([]byte, 2048)
		c.sAP, c.cAP = [32]byte{1}, [32]byte{2}
		var k trafficKeys
		if c.read.set(mem, c.sAP[:]) != nil || c.write.set(mem, c.cAP[:]) != nil || k.set(mem, c.sAP[:]) != nil {
			t.Fatal("no memory for the keys")
		}
		srv.keys = &k
		c.handshaked = true
		for i := 0; i < 100 && first == nil; i++ {
			var n int
			n, first = c.Read(buf)
			got = append(got, buf[:n]...)
		}
		k.close()
		c.read.close()
		c.write.close()
		c.rx, c.tx, c.closed = nil, nil, true
	} else {
		first = c.Handshake()
		if first == nil {
			var n int
			n, first = c.Read(buf)
			got = buf[:n]
		}
		c.Close()
	}
	if n := mem.Stats().InUse; n != 0 {
		t.Fatalf("memory in use after the run: %d", n)
	}
	return first, got
}

func FuzzClient(f *testing.F) {
	ch := newChain(f, "node.example")
	for _, s := range fuzzSeeds(ch) {
		f.Add(s)
	}
	heap := make([]byte, 48<<10)
	f.Fuzz(func(t *testing.T, data []byte) { runFuzz(t, ch, heap, data) })
}

// TestFuzzSeeds checks that the seeds go deep: the chain check passes and
// the signature fails in mode 1, and the data comes before the KeyUpdate in
// mode 2.
func TestFuzzSeeds(t *testing.T) {
	ch := newChain(t, "node.example")
	seeds := fuzzSeeds(ch)
	heap := make([]byte, 48<<10)
	if err, _ := runFuzz(t, ch, heap, seeds[0]); err != ErrSignature {
		t.Errorf("mode 1: %v, want %v", err, ErrSignature)
	}
	if err, _ := runFuzz(t, ch, heap, seeds[1]); err == nil {
		t.Errorf("mode 0: %v", err)
	}
	if err, got := runFuzz(t, ch, heap, seeds[2]); string(got) != "hello" || err == nil {
		t.Errorf("mode 2: %q %v", got, err)
	}
}
