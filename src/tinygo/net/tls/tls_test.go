package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	stdtls "crypto/tls"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"runtime/memory/arena"
)

type pipeConn struct{ net.Conn }

func (p pipeConn) SetReadDeadline(t time.Time) { p.Conn.SetReadDeadline(t) }

type chain struct {
	ca, inter  *ecdsa.PrivateKey
	caCert     *stdx509.Certificate
	interCert  *stdx509.Certificate
	serverCert stdtls.Certificate
}

// newChain makes a root CA, an intermediate CA and a leaf for host, all on
// P-256 with SHA-256.
func newChain(t testing.TB, host string) *chain {
	t.Helper()
	return newChainRoot(t, host, elliptic.P256(), stdx509.ECDSAWithSHA256)
}

// newChainRoot is newChain with a root key on curve, which signs the
// intermediate with alg.
func newChainRoot(t testing.TB, host string, curve elliptic.Curve, alg stdx509.SignatureAlgorithm) *chain {
	t.Helper()
	mk := func(tmpl, parent *stdx509.Certificate, pub *ecdsa.PublicKey, priv *ecdsa.PrivateKey) *stdx509.Certificate {
		der, err := stdx509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := stdx509.ParseCertificate(der)
		return c
	}
	now := time.Now()
	c := &chain{}
	c.ca, _ = ecdsa.GenerateKey(curve, rand.Reader)
	c.inter, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caT := &stdx509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "root"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign}
	c.caCert = mk(caT, caT, &c.ca.PublicKey, c.ca)
	inT := &stdx509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "inter"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign,
		SignatureAlgorithm: alg}
	c.interCert = mk(inT, c.caCert, &c.inter.PublicKey, c.ca)
	leafT := &stdx509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), SignatureAlgorithm: stdx509.ECDSAWithSHA256}
	leaf := mk(leafT, c.interCert, &leafKey.PublicKey, c.inter)
	c.serverCert = stdtls.Certificate{Certificate: [][]byte{leaf.Raw, c.interCert.Raw}, PrivateKey: leafKey}
	return c
}

func key(k *ecdsa.PrivateKey) PublicKey {
	var p PublicKey
	k.X.FillBytes(p.X[:])
	k.Y.FillBytes(p.Y[:])
	return p
}

// serve runs a crypto/tls server on the conn that sends a greeting, echoes
// one message and closes. It also sends NewSessionTicket after the handshake.
func serve(t *testing.T, conn net.Conn, ch *chain, done chan<- error) {
	serveCurve(t, conn, ch, stdtls.CurveP256, done)
}

// serveCurve is serve with only the key exchange group curve.
func serveCurve(t *testing.T, conn net.Conn, ch *chain, curve stdtls.CurveID, done chan<- error) {
	srv := stdtls.Server(conn, &stdtls.Config{
		Certificates:     []stdtls.Certificate{ch.serverCert},
		MinVersion:       stdtls.VersionTLS13,
		CurvePreferences: []stdtls.CurveID{curve},
	})
	if err := srv.Handshake(); err != nil {
		done <- err
		return
	}
	if _, err := srv.Write([]byte("hello client")); err != nil {
		done <- err
		return
	}
	buf := make([]byte, 64)
	n, err := srv.Read(buf)
	if err != nil {
		done <- err
		return
	}
	_, err = srv.Write(bytes.ToUpper(buf[:n]))
	if err == nil {
		err = srv.Close()
	}
	done <- err
}

// connPair gives the two ends of a TCP connection on the loopback interface.
// It has buffers, as on the board: after a HelloRetryRequest the server sends
// ChangeCipherSpec while the client sends the second ClientHello.
func connPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	a, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	b, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

func client(t *testing.T, cfg *Config) (*Conn, net.Conn, *arena.Heap) {
	a, b := connPair(t)
	t.Cleanup(func() { a.Close(); b.Close() })
	mem, _ := arena.NewHeap(make([]byte, 48<<10))
	return Client(pipeConn{a}, mem, rand.Reader, cfg), b, mem
}

func TestWithCryptoTLS(t *testing.T) {
	ch := newChain(t, "node.example")
	for _, name := range []string{"root anchor", "intermediate anchor", "pin"} {
		cfg := &Config{ServerName: "node.example", Time: time.Now()}
		switch name {
		case "root anchor":
			cfg.Roots = []PublicKey{key(ch.ca)}
		case "intermediate anchor":
			cfg.Roots = []PublicKey{key(ch.inter)}
		case "pin":
			leaf, _ := stdx509.ParseCertificate(ch.serverCert.Certificate[0])
			var pin [32]byte
			copy(pin[:], sha256Sum(leaf.RawSubjectPublicKeyInfo))
			cfg.Pins = [][32]byte{pin}
		}
		c, srvConn, mem := client(t, cfg)
		done := make(chan error, 1)
		go serve(t, srvConn, ch, done)
		if err := c.Handshake(); err != nil {
			t.Fatalf("%s: handshake: %v", name, err)
		}
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		if err != nil || string(buf[:n]) != "hello client" {
			t.Fatalf("%s: read %q %v", name, buf[:n], err)
		}
		if _, err := c.Write([]byte("echo me")); err != nil {
			t.Fatal(err)
		}
		n, err = c.Read(buf)
		if err != nil || string(buf[:n]) != "ECHO ME" {
			t.Fatalf("%s: echo %q %v", name, buf[:n], err)
		}
		if _, err := c.Read(buf); err != io.EOF {
			t.Fatalf("%s: after close_notify: %v, want EOF", name, err)
		}
		if err := <-done; err != nil {
			t.Fatalf("%s: server: %v", name, err)
		}
		c.Close()
		if mem.Stats().InUse != 0 {
			t.Errorf("%s: memory in use after Close: %d", name, mem.Stats().InUse)
		}
	}
}

// TestP384Root uses a P-384 root that signs the intermediate with SHA-384 or
// SHA-256.
func TestP384Root(t *testing.T) {
	for _, alg := range []stdx509.SignatureAlgorithm{stdx509.ECDSAWithSHA384, stdx509.ECDSAWithSHA256} {
		ch := newChainRoot(t, "node.example", elliptic.P384(), alg)
		anchor, err := AnchorFromCertificate(ch.caCert.Raw)
		if err != nil {
			t.Fatal(err)
		}
		var xy [96]byte
		ch.ca.X.FillBytes(xy[:48])
		ch.ca.Y.FillBytes(xy[48:])
		other, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		var bad [96]byte
		other.X.FillBytes(bad[:48])
		other.Y.FillBytes(bad[48:])
		for _, tc := range []struct {
			root PublicKey
			want error
		}{{anchor, nil}, {PublicKey{P384: xy[:]}, nil}, {PublicKey{P384: bad[:]}, ErrCertificate}} {
			c, srvConn, mem := client(t, &Config{ServerName: "node.example", Time: time.Now(), Roots: []PublicKey{tc.root}})
			done := make(chan error, 1)
			go serve(t, srvConn, ch, done)
			if err := c.Handshake(); err != tc.want {
				t.Fatalf("%v: handshake: %v, want %v", alg, err, tc.want)
			}
			srvConn.Close()
			<-done
			c.Close()
			if mem.Stats().InUse != 0 {
				t.Errorf("memory in use after Close: %d", mem.Stats().InUse)
			}
		}
	}
}

// TestRSA uses an RSA root, an RSA intermediate and an RSA leaf, so the
// chain has PKCS #1 v1.5 signatures and CertificateVerify uses PSS.
func TestRSA(t *testing.T) {
	now := time.Now()
	rootKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	interKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	leafKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	mk := func(tmpl, parent *stdx509.Certificate, pub, priv any) *stdx509.Certificate {
		der, err := stdx509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := stdx509.ParseCertificate(der)
		return c
	}
	ca := func(n int64, name string) *stdx509.Certificate {
		return &stdx509.Certificate{SerialNumber: big.NewInt(n), Subject: pkix.Name{CommonName: name}, NotBefore: now.Add(-time.Hour),
			NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign,
			SignatureAlgorithm: stdx509.SHA256WithRSA}
	}
	root := mk(ca(1, "rsa root"), ca(1, "rsa root"), &rootKey.PublicKey, rootKey)
	inter := mk(ca(2, "rsa inter"), root, &interKey.PublicKey, rootKey)
	leafT := &stdx509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "rsa.example"}, DNSNames: []string{"rsa.example"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), SignatureAlgorithm: stdx509.SHA256WithRSA}
	leaf := mk(leafT, inter, &leafKey.PublicKey, interKey)
	ch := &chain{serverCert: stdtls.Certificate{Certificate: [][]byte{leaf.Raw, inter.Raw}, PrivateKey: leafKey}}
	anchor, err := AnchorFromCertificate(root.Raw)
	if err != nil {
		t.Fatal(err)
	}
	c, srvConn, mem := client(t, &Config{ServerName: "rsa.example", Time: now, Roots: []PublicKey{anchor}})
	done := make(chan error, 1)
	go serve(t, srvConn, ch, done)
	if err := c.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	buf := make([]byte, 64)
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "hello client" {
		t.Fatalf("read %q %v", buf[:n], err)
	}
	c.Write([]byte("rsa"))
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "RSA" {
		t.Fatalf("echo %q %v", buf[:n], err)
	}
	c.Read(buf)
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
	c.Close()
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use after Close: %d", mem.Stats().InUse)
	}
}

func TestRejects(t *testing.T) {
	ch := newChain(t, "node.example")
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for _, tt := range []struct {
		name string
		cfg  Config
		want error
	}{
		{"unknown anchor", Config{ServerName: "node.example", Roots: []PublicKey{key(other)}}, ErrCertificate},
		{"other name", Config{ServerName: "evil.example", Roots: []PublicKey{key(ch.ca)}}, ErrName},
		{"expired", Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}, Time: time.Now().Add(48 * time.Hour)}, ErrExpired},
	} {
		c, srvConn, _ := client(t, &tt.cfg)
		done := make(chan error, 1)
		go serve(t, srvConn, ch, done)
		if err := c.Handshake(); err != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
		srvConn.Close()
		<-done
	}
}

// slowConn gives at most 3 bytes for each Read, and a timeout error for each
// second Read, as a transport with a short read deadline.
type slowConn struct {
	net.Conn
	n    int
	slow bool
}

type timeoutError struct{}

func (timeoutError) Error() string              { return "timeout" }
func (timeoutError) Timeout() bool              { return true }
func (s *slowConn) SetReadDeadline(t time.Time) {}
func (s *slowConn) Read(b []byte) (int, error) {
	if !s.slow {
		return s.Conn.Read(b)
	}
	s.n++
	if s.n%2 == 0 {
		return 0, timeoutError{}
	}
	return s.Conn.Read(b[:min(len(b), 3)])
}

// TestTimeouts reads, after the handshake, through a transport that times out
// in the middle of the records: the client must go on where it stopped.
func TestTimeouts(t *testing.T) {
	ch := newChain(t, "node.example")
	a, b := connPair(t)
	defer a.Close()
	defer b.Close()
	mem, _ := arena.NewHeap(make([]byte, 48<<10))
	sc := &slowConn{Conn: a}
	c := Client(sc, mem, rand.Reader, &Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}})
	done := make(chan error, 1)
	go serve(t, b, ch, done)
	if err := c.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	sc.slow = true
	buf := make([]byte, 64)
	var got []byte
	for len(got) < len("hello client") {
		n, err := c.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil && !isTimeout(err) {
			t.Fatal(err)
		}
	}
	if string(got) != "hello client" {
		t.Fatalf("read %q", got)
	}
	b.Close()
	<-done
}

func isTimeout(err error) bool {
	t, ok := err.(interface{ Timeout() bool })
	return ok && t.Timeout()
}

// TestX25519 uses a server that takes only X25519.
func TestX25519(t *testing.T) {
	ch := newChain(t, "node.example")
	c, srvConn, _ := client(t, &Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}})
	done := make(chan error, 1)
	go serveCurve(t, srvConn, ch, stdtls.X25519, done)
	if err := c.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	buf := make([]byte, 64)
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "hello client" {
		t.Fatalf("read %q %v", buf[:n], err)
	}
	c.Write([]byte("x"))
	c.Read(buf)
	c.Read(buf)
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
	c.Close()
}

// helloCounter counts the ClientHello records that the client writes.
type helloCounter struct {
	pipeConn
	hellos int
}

func (h *helloCounter) Write(b []byte) (int, error) {
	if len(b) > 5 && b[0] == recordHandshake && b[5] == hsClientHello {
		h.hellos++
	}
	return h.pipeConn.Write(b)
}

// TestHelloRetry checks that the client sends one key share for X25519, and
// a second ClientHello with secp256r1 when the server asks for it.
func TestHelloRetry(t *testing.T) {
	ch := newChain(t, "node.example")
	for _, tc := range []struct {
		curve  stdtls.CurveID
		hellos int
	}{{stdtls.X25519, 1}, {stdtls.CurveP256, 2}} {
		a, b := connPair(t)
		mem, _ := arena.NewHeap(make([]byte, 48<<10))
		hc := &helloCounter{pipeConn: pipeConn{a}}
		c := Client(hc, mem, rand.Reader, &Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}})
		done := make(chan error, 1)
		go serveCurve(t, b, ch, tc.curve, done)
		if err := c.Handshake(); err != nil {
			t.Fatalf("%v: handshake: %v", tc.curve, err)
		}
		buf := make([]byte, 64)
		if n, err := c.Read(buf); err != nil || string(buf[:n]) != "hello client" {
			t.Fatalf("%v: read %q %v", tc.curve, buf[:n], err)
		}
		if hc.hellos != tc.hellos {
			t.Errorf("%v: %d ClientHello messages, want %d", tc.curve, hc.hellos, tc.hellos)
		}
		b.Close()
		<-done
		c.Close()
		a.Close()
		if mem.Stats().InUse != 0 {
			t.Errorf("%v: memory in use after Close: %d", tc.curve, mem.Stats().InUse)
		}
	}
}
