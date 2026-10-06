package x509

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"math/big"
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

	// A P-384 CA and its SHA-384 signature are known but not supported.
	k384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	der384 := makeCert(t, caTmpl, caTmpl, &k384.PublicKey, k384)
	var c384 Certificate
	if err := Parse(der384, &c384); err != nil {
		t.Fatal(err)
	}
	if c384.P256 || c384.SigAlg != SigECDSASHA384 {
		t.Errorf("P-384 CA: P256 %v, SigAlg %d", c384.P256, c384.SigAlg)
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
}

func makeCertAny(t *testing.T, tmpl, parent *stdx509.Certificate, pub, priv any) []byte {
	t.Helper()
	der, err := stdx509.CreateCertificate(rand.Reader, tmpl, parent, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return der
}
