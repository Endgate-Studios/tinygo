package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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
	srv := stdtls.Server(conn, &stdtls.Config{
		Certificates:     []stdtls.Certificate{ch.serverCert},
		MinVersion:       stdtls.VersionTLS13,
		CurvePreferences: []stdtls.CurveID{stdtls.CurveP256},
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
