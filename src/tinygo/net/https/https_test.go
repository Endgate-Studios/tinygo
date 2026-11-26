package https

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	stdx509 "crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"runtime/memory/arena"
	"tinygo/net/tls"
)

type netConn struct{ net.Conn }

func (c netConn) SetReadDeadline(t time.Time) { c.Conn.SetReadDeadline(t) }

// testServer is an HTTPS server with TLS 1.3 and a certificate for
// example.test from a P-256 CA.
func testServer(t *testing.T, h http.Handler) (*httptest.Server, tls.PublicKey) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	caT := &stdx509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: stdx509.KeyUsageCertSign}
	caDER, _ := stdx509.CreateCertificate(rand.Reader, caT, caT, &caKey.PublicKey, caKey)
	ca, _ := stdx509.ParseCertificate(caDER)
	leafT := &stdx509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "example.test"}, DNSNames: []string{"example.test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), SignatureAlgorithm: stdx509.ECDSAWithSHA256}
	leafDER, err := stdx509.CreateCertificate(rand.Reader, leafT, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // the handshakes that the client stops
	srv.TLS = &stdtls.Config{MinVersion: stdtls.VersionTLS13,
		Certificates: []stdtls.Certificate{{Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafKey}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	anchor, err := tls.AnchorFromCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	return srv, anchor
}

func testClient(t *testing.T, srv *httptest.Server, anchor tls.PublicKey) (*Client, *arena.Heap) {
	mem, _ := arena.NewHeap(make([]byte, 48<<10))
	c := &Client{Mem: mem, Rand: rand.Reader, Now: time.Now,
		TLS: tls.Config{Roots: []tls.PublicKey{anchor}, Session: &tls.Session{}}}
	addr := srv.Listener.Addr().String()
	c.Dial = func(host string, port uint16, timeout time.Duration) (tls.RawConn, error) {
		if host != "example.test" || port != 8443 {
			t.Errorf("dial %s %d", host, port)
		}
		nc, err := net.DialTimeout("tcp", addr, timeout)
		return netConn{nc}, err
	}
	return c, mem
}

func TestGetAndPost(t *testing.T) {
	big := strings.Repeat("0123456789", 3000)
	srv, anchor := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chunked":
			w.Write([]byte("part one, "))
			w.(http.Flusher).Flush()
			w.Write([]byte("part two"))
		case "/big":
			w.Write([]byte(big))
		case "/echo":
			b, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
			w.WriteHeader(201)
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	c, mem := testClient(t, srv, anchor)
	for _, tc := range []struct {
		path, want string
		status     int
	}{
		{"/chunked", "part one, part two", 200},
		{"/big", big, 200},
		{"/none", "404 page not found\n", 404},
	} {
		var body bytes.Buffer
		status, err := c.Get("example.test:8443", tc.path, &body)
		if err != nil || status != tc.status || body.String() != tc.want {
			t.Errorf("GET %s: %d %v, %d bytes", tc.path, status, err, body.Len())
		}
	}
	var body bytes.Buffer
	status, err := c.Post("example.test:8443", "/echo", "application/json", []byte(`{"temp":21.5}`), &body)
	if err != nil || status != 201 || body.String() != `{"temp":21.5}` {
		t.Errorf("POST: %d %v %q", status, err, body.String())
	}
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use after the requests: %d", mem.Stats().InUse)
	}
}

func TestErrors(t *testing.T) {
	srv, _ := testServer(t, http.NotFoundHandler())
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	var wrong tls.PublicKey
	other.X.FillBytes(wrong.X[:])
	other.Y.FillBytes(wrong.Y[:])
	c, mem := testClient(t, srv, wrong)
	if _, err := c.Get("example.test:8443", "/", io.Discard); err != tls.ErrCertificate {
		t.Errorf("unknown CA: %v, want %v", err, tls.ErrCertificate)
	}
	for _, h := range []string{"example.test:", "example.test:0", "example.test:99999", "example.test:8x"} {
		if _, err := c.Get(h, "/", io.Discard); err != ErrHost {
			t.Errorf("%s: %v, want %v", h, err, ErrHost)
		}
	}
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use: %d", mem.Stats().InUse)
	}
}
