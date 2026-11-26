package web

import "testing"

// FuzzParseRequest gives request heads to the parser.
func FuzzParseRequest(f *testing.F) {
	f.Add([]byte("GET /index.html?a=1 HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\n\r\nhello"))
	f.Add([]byte("POST / HTTP/1.0\r\n\r\n"))
	f.Fuzz(func(t *testing.T, head []byte) {
		var r Request
		if parseRequest(&r, head) != nil {
			return
		}
		r.Header("Host")
		r.Header("Content-Length")
		r.PathIs("/")
	})
}
