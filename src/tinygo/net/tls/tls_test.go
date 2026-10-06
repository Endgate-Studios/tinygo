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
func newChain(t *testing.T, host string) *chain {
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
	c.ca, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	c.inter, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caT := &stdx509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "root"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign}
	c.caCert = mk(caT, caT, &c.ca.PublicKey, c.ca)
	inT := &stdx509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "inter"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign,
		SignatureAlgorithm: stdx509.ECDSAWithSHA256}
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

func client(t *testing.T, cfg *Config) (*Conn, net.Conn, *arena.Heap) {
	a, b := net.Pipe()
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
	a, b := net.Pipe()
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
