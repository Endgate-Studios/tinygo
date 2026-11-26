package tls

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	stdtls "crypto/tls"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"io"
	"math/big"
	"testing"
	"time"
)

// talk12 does what serveConfig's client side expects: reads the greeting,
// sends a message, reads it back in capitals and then close_notify.
func talk12(t *testing.T, name string, c *Conn) {
	t.Helper()
	buf := make([]byte, 64)
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "hello client" {
		t.Fatalf("%s: read %q %v", name, buf[:n], err)
	}
	if _, err := c.Write([]byte("over tls 1.2")); err != nil {
		t.Fatalf("%s: write: %v", name, err)
	}
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "OVER TLS 1.2" {
		t.Fatalf("%s: echo %q %v", name, buf[:n], err)
	}
	if _, err := c.Read(buf); err != io.EOF {
		t.Fatalf("%s: after close_notify: %v, want EOF", name, err)
	}
}

// TestTLS12 talks to a crypto/tls server that has only TLS 1.2, with an
// ECDSA chain, ECDHE on each curve, and one that asks for a client
// certificate.
func TestTLS12(t *testing.T) {
	ch := newChain(t, "node.example")
	for _, tc := range []struct {
		name  string
		curve stdtls.CurveID
		auth  stdtls.ClientAuthType
		suite uint16
	}{
		{"x25519", stdtls.X25519, stdtls.NoClientCert, stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		{"secp256r1", stdtls.CurveP256, stdtls.NoClientCert, stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		{"asks for a client certificate", stdtls.X25519, stdtls.RequestClientCert, stdtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
	} {
		c, srvConn, mem := client(t, &Config{ServerName: "node.example", Time: time.Now(), Roots: []PublicKey{key(ch.ca)}, Session: &Session{}})
		done := make(chan error, 1)
		var state stdtls.ConnectionState
		go serveConfig(srvConn, &stdtls.Config{
			Certificates:     []stdtls.Certificate{ch.serverCert},
			MaxVersion:       stdtls.VersionTLS12,
			CurvePreferences: []stdtls.CurveID{tc.curve},
			ClientAuth:       tc.auth,
			VerifyConnection: func(s stdtls.ConnectionState) error { state = s; return nil },
		}, done)
		if err := c.Handshake(); err != nil {
			t.Fatalf("%s: handshake: %v", tc.name, err)
		}
		talk12(t, tc.name, c)
		if err := <-done; err != nil {
			t.Fatalf("%s: server: %v", tc.name, err)
		}
		if state.Version != stdtls.VersionTLS12 || state.CipherSuite != tc.suite || state.CurveID != tc.curve {
			t.Errorf("%s: version %x suite %x curve %v", tc.name, state.Version, state.CipherSuite, state.CurveID)
		}
		if !c.v12 {
			t.Errorf("%s: the client didn't take the 1.2 path", tc.name)
		}
		c.Close()
		if mem.Stats().InUse != 0 {
			t.Errorf("%s: memory in use after Close: %d", tc.name, mem.Stats().InUse)
		}
	}
}

// TestTLS12RSA talks TLS 1.2 to a server with an RSA chain: ECDHE_RSA, its
// key exchange signed with the leaf's RSA key.
func TestTLS12RSA(t *testing.T) {
	now := time.Now()
	rootKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	leafKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	mk := func(tmpl, parent *stdx509.Certificate, pub, priv any) *stdx509.Certificate {
		der, err := stdx509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := stdx509.ParseCertificate(der)
		return c
	}
	rootT := &stdx509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "rsa root"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign}
	root := mk(rootT, rootT, &rootKey.PublicKey, rootKey)
	leafT := &stdx509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "rsa.example"}, DNSNames: []string{"rsa.example"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	leaf := mk(leafT, root, &leafKey.PublicKey, rootKey)
	anchor, err := AnchorFromCertificate(root.Raw)
	if err != nil {
		t.Fatal(err)
	}
	c, srvConn, mem := client(t, &Config{ServerName: "rsa.example", Time: now, Roots: []PublicKey{anchor}})
	done := make(chan error, 1)
	go serveConfig(srvConn, &stdtls.Config{
		Certificates: []stdtls.Certificate{{Certificate: [][]byte{leaf.Raw}, PrivateKey: leafKey}},
		MaxVersion:   stdtls.VersionTLS12,
	}, done)
	if err := c.Handshake(); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	talk12(t, "rsa", c)
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
	c.Close()
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use after Close: %d", mem.Stats().InUse)
	}
}

// TestTLS12Rejects: the checks of the chain hold for 1.2 as for 1.3.
func TestTLS12Rejects(t *testing.T) {
	ch := newChain(t, "node.example")
	other := newChain(t, "node.example")
	for _, tt := range []struct {
		name string
		cfg  Config
		want error
	}{
		{"unknown anchor", Config{ServerName: "node.example", Roots: []PublicKey{key(other.ca)}}, ErrCertificate},
		{"other name", Config{ServerName: "evil.example", Roots: []PublicKey{key(ch.ca)}}, ErrName},
		{"expired", Config{ServerName: "node.example", Roots: []PublicKey{key(ch.ca)}, Time: time.Now().Add(48 * time.Hour)}, ErrExpired},
	} {
		c, srvConn, _ := client(t, &tt.cfg)
		done := make(chan error, 1)
		go serveConfig(srvConn, &stdtls.Config{Certificates: []stdtls.Certificate{ch.serverCert}, MaxVersion: stdtls.VersionTLS12}, done)
		if err := c.Handshake(); err != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
		srvConn.Close()
		<-done
		c.Close()
	}
}

// TestTLS12Downgrade: a 1.2 ServerHello whose random ends as a 1.3 server's
// does when it answers a client without 1.3 is refused.
func TestTLS12Downgrade(t *testing.T) {
	var st hsState
	var random [32]byte
	copy(random[24:], "DOWNGRD\x01")
	var x, y [32]byte
	for _, last := range []byte{0, 1} {
		random[31] = last
		sh := []byte{3, 3}
		sh = append(sh, random[:]...)
		sh = append(sh, 0, 0xc0, 0x2b, 0)
		if _, err := parseServerHello(sh, &st, &x, &y, 0); err != ErrDowngrade {
			t.Errorf("DOWNGRD %d: %v, want ErrDowngrade", last, err)
		}
	}
	random[31] = 2
	sh := append(append([]byte{3, 3}, random[:]...), 0, 0xc0, 0x2b, 0)
	if _, err := parseServerHello(sh, &st, &x, &y, 0); err != nil || !st.v12 {
		t.Errorf("an ordinary 1.2 ServerHello: %v, v12 %v", err, st.v12)
	}
	sh = append(append([]byte{3, 1}, random[:]...), 0, 0xc0, 0x2b, 0)
	if _, err := parseServerHello(sh, &st, &x, &y, 0); err != ErrUnsupported {
		t.Errorf("TLS 1.0: %v, want ErrUnsupported", err)
	}
}

// TestPRF checks the TLS 1.2 PRF with SHA-256 against the test vector that's
// gone round since RFC 5246.
func TestPRF(t *testing.T) {
	secret, _ := hex.DecodeString("9bbe436ba940f017b17652849a71db35")
	seed, _ := hex.DecodeString("a0ba9f936cda311827a6f796ffd5198c")
	want, _ := hex.DecodeString("e3f229ba727be17b8d122620557cd453c2aab21d07c3d495329b52d4e61edb5a" +
		"6b301791e90d35c9c9a46b4e14baf9af0fa022f7077def17abfd3797c0564bab" +
		"4fbc91666e9def9b97fce34f796789baa48082d122ee42c5a72e5a5110fff701" +
		"87347b66")
	got := make([]byte, len(want))
	prf(got, secret, "test label", seed, nil)
	if !bytes.Equal(got, want) {
		t.Errorf("prf:\n got %x\nwant %x", got, want)
	}
}
