package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"runtime/memory/arena"
)

func makeCert(t *testing.T, tmpl, parent *stdx509.Certificate, pub *ecdsa.PublicKey, priv *ecdsa.PrivateKey) []byte {
	t.Helper()
	der, err := stdx509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func TestParseAndVerify(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 8<<10))
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &stdx509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2051, 6, 1, 12, 30, 15, 0, time.UTC), // GeneralizedTime
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              stdx509.KeyUsageCertSign,
	}
	caDER := makeCert(t, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &stdx509.Certificate{
		SerialNumber:       big.NewInt(2),
		Subject:            pkix.Name{CommonName: "node"},
		NotBefore:          time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		NotAfter:           time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC),
		DNSNames:           []string{"example.com", "*.Example.com"},
		SignatureAlgorithm: stdx509.ECDSAWithSHA256,
	}
	leafDER := makeCert(t, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)

	var ca, leaf Certificate
	if err := Parse(caDER, &ca); err != nil {
		t.Fatal(err)
	}
	if err := Parse(leafDER, &leaf); err != nil {
		t.Fatal(err)
	}
	if !ca.IsCA || leaf.IsCA || !ca.P256 || !leaf.P256 {
		t.Fatalf("CA flags %v %v, P-256 %v %v", ca.IsCA, leaf.IsCA, ca.P256, leaf.P256)
	}
	if !ca.NotAfter.Equal(caTmpl.NotAfter) || !leaf.NotBefore.Equal(leafTmpl.NotBefore) || !leaf.NotAfter.Equal(leafTmpl.NotAfter) {
		t.Errorf("times %v %v %v", ca.NotAfter, leaf.NotBefore, leaf.NotAfter)
	}
	if leaf.KeyX != [32]byte(leafKey.X.FillBytes(make([]byte, 32))) {
		t.Error("leaf key differs")
	}
	if !leaf.CheckSignatureFrom(mem, &ca) {
		t.Fatal("the leaf signature of the CA does not verify")
	}
	if leaf.CheckSignatureFrom(mem, &leaf) {
		t.Error("the leaf verifies as its own CA")
	}
	for host, want := range map[string]bool{
		"example.com": true, "EXAMPLE.com": true, "www.example.com": true,
		"a.b.example.com": false, "example.org": false, "badexample.com": false, "": false,
	} {
		if got := leaf.VerifyHostname(host); got != want {
			t.Errorf("host %q: %v, want %v", host, got, want)
		}
	}
	// A changed signed part does not verify.
	bad := append([]byte(nil), leafDER...)
	var tampered Certificate
	Parse(bad, &tampered)
	tampered.TBS[len(tampered.TBS)-1] ^= 1
	if tampered.CheckSignatureFrom(mem, &ca) {
		t.Error("a changed certificate verifies")
	}

	// A P-384 CA signs with SHA-384 or SHA-256, and a P-256 CA with SHA-384.
	k384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der384 := makeCert(t, caTmpl, caTmpl, &k384.PublicKey, k384)
	var c384 Certificate
	if err := Parse(der384, &c384); err != nil {
		t.Fatal(err)
	}
	if c384.P256 || !c384.P384 || c384.SigAlg != SigECDSASHA384 {
		t.Errorf("P-384 CA: P256 %v, P384 %v, SigAlg %d", c384.P256, c384.P384, c384.SigAlg)
	}
	if !c384.CheckSignatureFrom(mem, &c384) {
		t.Error("the P-384 CA does not verify itself")
	}
	for _, alg := range []stdx509.SignatureAlgorithm{stdx509.ECDSAWithSHA384, stdx509.ECDSAWithSHA256} {
		leafTmpl.SignatureAlgorithm = alg
		var l Certificate
		if err := Parse(makeCert(t, leafTmpl, caTmpl, &leafKey.PublicKey, k384), &l); err != nil {
			t.Fatal(err)
		}
		if !l.CheckSignatureFrom(mem, &c384) {
			t.Errorf("%v from the P-384 CA does not verify", alg)
		}
		if l.CheckSignatureFrom(mem, &ca) {
			t.Errorf("%v from the P-384 CA verifies with the P-256 CA", alg)
		}
	}
	leafTmpl.SignatureAlgorithm = stdx509.ECDSAWithSHA384
	var l384 Certificate
	if err := Parse(makeCert(t, leafTmpl, caTmpl, &leafKey.PublicKey, caKey), &l384); err != nil {
		t.Fatal(err)
	}
	if !l384.CheckSignatureFrom(mem, &ca) || l384.CheckSignatureFrom(mem, &c384) {
		t.Error("SHA-384 from the P-256 CA")
	}
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use: %d", mem.Stats().InUse)
	}
}

func TestParseBad(t *testing.T) {
	var c Certificate
	for _, b := range [][]byte{nil, {0x30}, {0x30, 0x82, 0xff}, {0x30, 0x03, 0x30, 0x01, 0x00}} {
		if Parse(b, &c) == nil {
			t.Errorf("%x parsed", b)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte{0x30, 0x03, 0x30, 0x01, 0x00})
	f.Fuzz(func(t *testing.T, b []byte) {
		var c Certificate
		if Parse(b, &c) == nil {
			c.VerifyHostname("example.com")
		}
	})
}

func TestRSA(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 16<<10))
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	caTmpl := &stdx509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "RSA CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: stdx509.KeyUsageCertSign, SignatureAlgorithm: stdx509.SHA256WithRSA}
	caDER := makeCertAny(t, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	leafKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	leafTmpl := &stdx509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "leaf"}, DNSNames: []string{"rsa.example"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), SignatureAlgorithm: stdx509.SHA256WithRSA}
	leafDER := makeCertAny(t, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	var ca, leaf Certificate
	if err := Parse(caDER, &ca); err != nil {
		t.Fatal(err)
	}
	if err := Parse(leafDER, &leaf); err != nil {
		t.Fatal(err)
	}
	if !ca.RSA || !leaf.RSA || leaf.SigAlg != SigRSASHA256 || leaf.RSAKey.E != 65537 || len(leaf.RSAKey.N) != 256 {
		t.Fatalf("RSA %v %v, alg %d, e %d, n %d bytes", ca.RSA, leaf.RSA, leaf.SigAlg, leaf.RSAKey.E, len(leaf.RSAKey.N))
	}
	if !leaf.CheckSignatureFrom(mem, &ca) {
		t.Fatal("the leaf signature of the RSA CA does not verify")
	}
	leaf.TBS[len(leaf.TBS)-1] ^= 1
	if leaf.CheckSignatureFrom(mem, &ca) {
		t.Error("a changed certificate verifies")
	}
	for _, tc := range []struct {
		alg  stdx509.SignatureAlgorithm
		want int
	}{{stdx509.SHA384WithRSA, SigRSASHA384}, {stdx509.SHA512WithRSA, SigRSASHA512}} {
		leafTmpl.SignatureAlgorithm = tc.alg
		var l Certificate
		if err := Parse(makeCertAny(t, leafTmpl, caTmpl, &leafKey.PublicKey, caKey), &l); err != nil {
			t.Fatal(err)
		}
		if l.SigAlg != tc.want || !l.CheckSignatureFrom(mem, &ca) {
			t.Errorf("%v: SigAlg %d, does not verify", tc.alg, l.SigAlg)
		}
	}
}

func makeCertAny(t *testing.T, tmpl, parent *stdx509.Certificate, pub, priv any) []byte {
	t.Helper()
	der, err := stdx509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// TestUsage checks keyUsage, extKeyUsage, pathLenConstraint and an unknown
// critical extension.
func TestUsage(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 8<<10))
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	base := func(n int64, name string) *stdx509.Certificate {
		return &stdx509.Certificate{SerialNumber: big.NewInt(n), Subject: pkix.Name{CommonName: name},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	}
	parse := func(tmpl, parent *stdx509.Certificate) (Certificate, error) {
		var c Certificate
		err := Parse(makeCert(t, tmpl, parent, &key.PublicKey, key), &c)
		return c, err
	}

	ca := base(1, "ca")
	ca.IsCA, ca.BasicConstraintsValid, ca.KeyUsage = true, true, stdx509.KeyUsageCertSign
	ca.MaxPathLen, ca.MaxPathLenZero = 0, true
	c, err := parse(ca, ca)
	if err != nil || !c.CanSign() || c.MaxPathLen != 0 || !c.HasKeyUsage || c.KeyUsage != UsageCertSign {
		t.Fatalf("CA: %v, CanSign %v, MaxPathLen %d, KeyUsage %#x", err, c.CanSign(), c.MaxPathLen, c.KeyUsage)
	}
	ca.MaxPathLen, ca.MaxPathLenZero = 300, false
	if c, _ = parse(ca, ca); c.MaxPathLen != 300 {
		t.Errorf("MaxPathLen %d, want 300", c.MaxPathLen)
	}
	ca.MaxPathLen = -1
	if c, _ = parse(ca, ca); c.MaxPathLen != -1 {
		t.Errorf("MaxPathLen %d, want -1", c.MaxPathLen)
	}
	signer := c

	// A CA without keyCertSign must not sign.
	noSign := base(2, "ca")
	noSign.IsCA, noSign.BasicConstraintsValid, noSign.KeyUsage = true, true, stdx509.KeyUsageDigitalSignature
	c, _ = parse(noSign, noSign)
	if c.CanSign() {
		t.Error("a CA without keyCertSign can sign")
	}
	leafTmpl := base(3, "leaf")
	leafTmpl.SignatureAlgorithm = stdx509.ECDSAWithSHA256
	leaf, _ := parse(leafTmpl, noSign)
	if leaf.CheckSignatureFrom(mem, &c) {
		t.Error("a CA without keyCertSign signed")
	}
	if leaf, _ = parse(leafTmpl, ca); !leaf.CheckSignatureFrom(mem, &signer) {
		t.Error("the CA with keyCertSign did not sign")
	}

	for _, tc := range []struct {
		ku   stdx509.KeyUsage
		eku  []stdx509.ExtKeyUsage
		want bool
	}{
		{0, nil, true},
		{stdx509.KeyUsageDigitalSignature, []stdx509.ExtKeyUsage{stdx509.ExtKeyUsageServerAuth}, true},
		{stdx509.KeyUsageDigitalSignature, []stdx509.ExtKeyUsage{stdx509.ExtKeyUsageAny}, true},
		{stdx509.KeyUsageKeyEncipherment, nil, false},
		{0, []stdx509.ExtKeyUsage{stdx509.ExtKeyUsageClientAuth}, false},
		{stdx509.KeyUsageDigitalSignature, []stdx509.ExtKeyUsage{stdx509.ExtKeyUsageClientAuth, stdx509.ExtKeyUsageServerAuth}, true},
	} {
		l := base(4, "leaf")
		l.KeyUsage, l.ExtKeyUsage = tc.ku, tc.eku
		c, err := parse(l, ca)
		if err != nil || c.ForServer() != tc.want {
			t.Errorf("key usage %#x, extended %v: %v, ForServer %v, want %v", tc.ku, tc.eku, err, c.ForServer(), tc.want)
		}
	}

	// An unknown critical extension, and the same one not critical.
	crit := base(5, "leaf")
	crit.ExtraExtensions = []pkix.Extension{{Id: []int{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0}}}
	if _, err := parse(crit, ca); err != ErrCritical {
		t.Errorf("critical extension: %v, want %v", err, ErrCritical)
	}
	crit.ExtraExtensions[0].Critical = false
	if _, err := parse(crit, ca); err != nil {
		t.Errorf("extension that is not critical: %v", err)
	}
}

func TestParseIPv4(t *testing.T) {
	for _, tc := range []struct {
		s  string
		ip [4]byte
		ok bool
	}{
		{"192.168.1.10", [4]byte{192, 168, 1, 10}, true},
		{"0.0.0.0", [4]byte{}, true},
		{"255.255.255.255", [4]byte{255, 255, 255, 255}, true},
		{"256.1.1.1", [4]byte{}, false},
		{"1.2.3", [4]byte{}, false},
		{"1.2.3.4.5", [4]byte{}, false},
		{"01.2.3.4", [4]byte{}, false},
		{"1..3.4", [4]byte{}, false},
		{"1.2.3.4.", [4]byte{}, false},
		{"example.com", [4]byte{}, false},
		{"", [4]byte{}, false},
	} {
		ip, ok := ParseIPv4(tc.s)
		if ok != tc.ok || (ok && ip != tc.ip) {
			t.Errorf("%q: %v %v, want %v %v", tc.s, ip, ok, tc.ip, tc.ok)
		}
	}
}

// TestIPAddress checks that an address matches only an iPAddress.
func TestIPAddress(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &stdx509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "device"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"device.local", "10.0.0.2"}, IPAddresses: []net.IP{net.IPv4(192, 168, 1, 10), net.ParseIP("2001:db8::1")}}
	var c Certificate
	if err := Parse(makeCert(t, tmpl, tmpl, &key.PublicKey, key), &c); err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]bool{
		"192.168.1.10": true,
		"192.168.1.11": false,
		"10.0.0.2":     false, // only a dNSName
		"device.local": true,
		"2001:db8::1":  false,
	} {
		if got := c.VerifyHostname(host); got != want {
			t.Errorf("%s: %v, want %v", host, got, want)
		}
	}
}
