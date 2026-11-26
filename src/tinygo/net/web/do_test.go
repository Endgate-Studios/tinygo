package web

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// scriptConn gives the response in reads of at most 3 bytes and keeps the
// request.
type scriptConn struct {
	resp []byte
	req  bytes.Buffer
}

func (c *scriptConn) Write(b []byte) (int, error) { return c.req.Write(b) }

func (c *scriptConn) Read(b []byte) (int, error) {
	if len(c.resp) == 0 {
		return 0, io.EOF
	}
	n := copy(b[:min(len(b), 3)], c.resp)
	c.resp = c.resp[n:]
	return n, nil
}

func TestDo(t *testing.T) {
	for _, tc := range []struct {
		name, method, resp string
		status             int
		body               string
		err                error
	}{
		{"length", "GET", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhelloEXTRA", 200, "hello", nil},
		{"short", "GET", "HTTP/1.1 200 OK\r\nContent-Length: 9\r\n\r\nhello", 200, "hello", io.ErrUnexpectedEOF},
		{"chunked", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n7;ext=1\r\n, world\r\n0\r\nX-Trailer: 1\r\n\r\nEXTRA", 200, "hello, world", nil},
		{"chunked upper", "GET", "HTTP/1.1 200 OK\r\ntransfer-encoding: Chunked\r\n\r\nA\r\n0123456789\r\n0\r\n\r\n", 200, "0123456789", nil},
		{"until close", "GET", "HTTP/1.1 404 Not Found\r\n\r\nnot here", 404, "not here", nil},
		{"head", "HEAD", "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n", 200, "", nil},
		{"bad size", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n", 200, "", ErrBadBody},
		{"no CRLF after chunk", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nabX\r\n0\r\n\r\n", 200, "ab", ErrBadBody},
		{"cut chunked", "GET", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhel", 200, "hel", io.ErrUnexpectedEOF},
		{"bad length", "GET", "HTTP/1.1 200 OK\r\nContent-Length: x\r\n\r\n", 200, "", ErrBadHead},
		{"bad status", "GET", "HTTP/1.1 2x0 OK\r\n\r\n", 0, "", ErrBadHead},
	} {
		c := &scriptConn{resp: []byte(tc.resp)}
		var body bytes.Buffer
		status, err := Do(c, tc.method, "example.com", "/x", "", nil, make([]byte, 128), &body)
		if status != tc.status || err != tc.err || body.String() != tc.body {
			t.Errorf("%s: %d %v %q, want %d %v %q", tc.name, status, err, body.String(), tc.status, tc.err, tc.body)
		}
	}
}

// DoRedirect gives a redirect's Location and leaves its body, and is Do for anything else.
func TestDoRedirect(t *testing.T) {
	for _, tc := range []struct {
		name, resp     string
		status         int
		location, body string
		err            error
		room           int
	}{
		{"moved", "HTTP/1.1 301 Moved Permanently\r\nLocation: https://example.com/new\r\nContent-Length: 5\r\n\r\nmoved", 301, "https://example.com/new", "", nil, 64},
		{"relative", "HTTP/1.1 302 Found\r\nlocation: /other\r\n\r\n", 302, "/other", "", nil, 64},
		{"none", "HTTP/1.1 303 See Other\r\n\r\n", 303, "", "", nil, 64},
		{"too long", "HTTP/1.1 307 Temporary Redirect\r\nLocation: https://example.com/a/long/way\r\n\r\n", 307, "", "", ErrHeadTooLarge, 8},
		{"not a redirect", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello", 200, "", "hello", nil, 64},
	} {
		c := &scriptConn{resp: []byte(tc.resp)}
		var body bytes.Buffer
		location := make([]byte, tc.room)
		status, n, err := DoRedirect(c, "GET", "example.com", "/x", "", nil, make([]byte, 128), &body, location)
		if status != tc.status || err != tc.err || string(location[:n]) != tc.location || body.String() != tc.body {
			t.Errorf("%s: %d %v %q %q, want %d %v %q %q", tc.name, status, err, location[:n], body.String(), tc.status, tc.err, tc.location, tc.body)
		}
	}
}

func TestDoRequest(t *testing.T) {
	c := &scriptConn{resp: []byte("HTTP/1.1 201 Created\r\nContent-Length: 0\r\n\r\n")}
	status, err := Do(c, "POST", "example.com", "/api", "Content-Type: application/json\r\n", []byte(`{"a":1}`), make([]byte, 256), io.Discard)
	want := "POST /api HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\nContent-Type: application/json\r\nContent-Length: 7\r\n\r\n{\"a\":1}"
	if status != 201 || err != nil || c.req.String() != want {
		t.Errorf("%d %v %q, want %q", status, err, c.req.String(), want)
	}
	if _, err := Do(c, "GET", "example.com", strings.Repeat("a", 300), "", nil, make([]byte, 256), io.Discard); err != ErrHeadTooLarge {
		t.Errorf("long path: %v, want %v", err, ErrHeadTooLarge)
	}
}

func FuzzDo(f *testing.F) {
	f.Add([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"))
	f.Add([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	f.Fuzz(func(t *testing.T, resp []byte) {
		Do(&scriptConn{resp: resp}, "GET", "h", "/", "", nil, make([]byte, 64), io.Discard)
	})
}
