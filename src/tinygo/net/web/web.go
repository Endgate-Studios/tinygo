// Package web is a small HTTP/1.1 server and client on tinygo/net/ip that does
// not allocate for each request. Requests are parsed in place in a buffer of
// the caller, and each response ends with Connection: close.
package web

import (
	"errors"
	"io"
	"strconv"
	"time"
	"unsafe"

	"tinygo/net/ip"
)

var (
	ErrHeadTooLarge = errors.New("web: request or response head larger than the buffer")
	ErrBadHead      = errors.New("web: bad HTTP head")
)

// ReadTimeout is how long the server waits for a request head.
const ReadTimeout = 5 * time.Second

// Request is an HTTP request. Its slices point into the buffer of the server
// and are valid only while the handler runs.
type Request struct {
	Method []byte
	Path   []byte // without the query
	Query  []byte // after '?', without it
	header []byte // the header lines
	body   []byte // body bytes that came with the head
	remain int    // body bytes still on the connection
	conn   *ip.TCPConn
}

// PathIs reports whether the path is p. string(r.Path) == p can allocate.
func (r *Request) PathIs(p string) bool { return equal(r.Path, p) }

// Header gives the value of the header name, or nil. The name is not case
// sensitive.
func (r *Request) Header(name string) []byte {
	return findHeader(r.header, name)
}

// Read reads the body, up to Content-Length.
func (r *Request) Read(p []byte) (int, error) {
	if len(r.body) > 0 {
		n := copy(p, r.body)
		r.body = r.body[n:]
		return n, nil
	}
	if r.remain <= 0 {
		return 0, io.EOF
	}
	n, err := r.conn.Read(p[:min(len(p), r.remain)])
	r.remain -= n
	return n, err
}

// Response writes an HTTP response. It collects small writes and sends them
// as one segment.
type Response struct {
	conn    *ip.TCPConn
	started bool
	err     error
	n       int
	num     [20]byte // scratch for numbers
	out     [512]byte
}

// WriteHeader writes the status line and the headers. Write calls it with
// 200 and text/plain when the handler did not.
func (w *Response) WriteHeader(status int, contentType string) {
	if w.started {
		return
	}
	w.started = true
	w.put("HTTP/1.1 ")
	w.putBytes(strconv.AppendInt(w.num[:0], int64(status), 10))
	w.put(" ")
	w.put(statusText(status))
	w.put("\r\nContent-Type: ")
	w.put(contentType)
	w.put("\r\nConnection: close\r\n\r\n")
}

// Write writes body bytes.
func (w *Response) Write(p []byte) (int, error) {
	if !w.started {
		w.WriteHeader(200, "text/plain")
	}
	w.putBytes(p)
	return len(p), w.err
}

// WriteInt writes n in decimal.
func (w *Response) WriteInt(n int64) (int, error) {
	return w.Write(strconv.AppendInt(w.num[:0], n, 10))
}

// WriteUint writes n in decimal.
func (w *Response) WriteUint(n uint64) (int, error) {
	return w.Write(strconv.AppendUint(w.num[:0], n, 10))
}

// WriteString writes body text without a copy to a byte slice.
func (w *Response) WriteString(s string) (int, error) {
	return w.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
}

func (w *Response) put(s string) { w.putBytes(unsafe.Slice(unsafe.StringData(s), len(s))) }

func (w *Response) putBytes(p []byte) {
	for len(p) > 0 && w.err == nil {
		if w.n == len(w.out) {
			w.flush()
		}
		c := copy(w.out[w.n:], p)
		w.n += c
		p = p[c:]
	}
}

func (w *Response) flush() {
	if w.n > 0 && w.err == nil {
		_, w.err = w.conn.Write(w.out[:w.n])
	}
	w.n = 0
}

// Handler answers a request.
type Handler func(w *Response, r *Request)

// Server serves HTTP on a listener. Make it once: it holds the request and
// the response, so a request allocates nothing.
type Server struct {
	l   *ip.TCPListener
	h   Handler
	buf []byte
	req Request
	res Response
}

// NewServer makes a server. buf holds each request head, and a larger head
// gets status 431.
func NewServer(l *ip.TCPListener, buf []byte, h Handler) *Server {
	return &Server{l: l, h: h, buf: buf}
}

// Serve answers one connection at a time until the listener closes.
func (s *Server) Serve() error {
	for {
		c, err := s.l.Accept()
		if err != nil {
			return err
		}
		s.serve(c)
		c.Close()
	}
}

func (s *Server) serve(c *ip.TCPConn) {
	r, w := &s.req, &s.res
	// Field by field: a Response literal is too large for the stack and
	// would be allocated.
	w.conn, w.started, w.err, w.n = c, false, nil, 0
	c.SetReadDeadline(time.Now().Add(ReadTimeout))
	head, rest, err := readHead(c, s.buf)
	c.SetReadDeadline(time.Time{})
	if err == ErrHeadTooLarge {
		w.WriteHeader(431, "text/plain")
		w.flush()
		return
	}
	if err != nil || parseRequest(r, head) != nil {
		if err == nil {
			w.WriteHeader(400, "text/plain")
			w.flush()
		}
		return
	}
	r.conn, r.body = c, rest
	r.remain = 0
	if n, ok := atoi(r.Header("Content-Length")); ok {
		r.remain = n - len(rest)
		if r.remain < 0 {
			r.body, r.remain = rest[:n], 0
		}
	} else {
		r.body = nil
	}
	s.h(w, r)
	if !w.started {
		w.WriteHeader(204, "text/plain")
	}
	w.flush()
}

// readHead reads until the empty line. It gives the head without the empty
// line, and the bytes after it.
func readHead(c *ip.TCPConn, buf []byte) (head, rest []byte, err error) {
	n := 0
	for {
		if i := indexHeadEnd(buf[:n]); i >= 0 {
			return buf[:i], buf[i+4 : n], nil
		}
		if n == len(buf) {
			return nil, nil, ErrHeadTooLarge
		}
		m, err := c.Read(buf[n:])
		n += m
		if err != nil && m == 0 {
			return nil, nil, err
		}
	}
}

func indexHeadEnd(b []byte) int {
	for i := 0; i+3 < len(b); i++ {
		if b[i] == '\r' && b[i+1] == '\n' && b[i+2] == '\r' && b[i+3] == '\n' {
			return i
		}
	}
	return -1
}

// parseRequest parses "METHOD target HTTP/1.x" and keeps the header lines.
func parseRequest(r *Request, head []byte) error {
	line, header := cutLine(head)
	sp1 := indexByte(line, ' ')
	if sp1 <= 0 {
		return ErrBadHead
	}
	sp2 := indexByte(line[sp1+1:], ' ')
	if sp2 <= 0 {
		return ErrBadHead
	}
	target := line[sp1+1 : sp1+1+sp2]
	r.Method, r.Path, r.Query, r.header = line[:sp1], target, nil, header
	if q := indexByte(target, '?'); q >= 0 {
		r.Path, r.Query = target[:q], target[q+1:]
	}
	return nil
}

func cutLine(b []byte) (line, rest []byte) {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '\r' && b[i+1] == '\n' {
			return b[:i], b[i+2:]
		}
	}
	return b, nil
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// findHeader looks for "name: value" in the header lines.
func findHeader(header []byte, name string) []byte {
	for len(header) > 0 {
		var line []byte
		line, header = cutLine(header)
		colon := indexByte(line, ':')
		if colon != len(name) || !equalFold(line[:colon], name) {
			continue
		}
		v := line[colon+1:]
		for len(v) > 0 && (v[0] == ' ' || v[0] == '\t') {
			v = v[1:]
		}
		for len(v) > 0 && (v[len(v)-1] == ' ' || v[len(v)-1] == '\t') {
			v = v[:len(v)-1]
		}
		return v
	}
	return nil
}

func equal(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

func equalFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		x, y := b[i], s[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func atoi(b []byte) (int, bool) {
	if len(b) == 0 || len(b) > 9 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func statusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 204:
		return "No Content"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 400:
		return "Bad Request"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 431:
		return "Request Header Fields Too Large"
	case 500:
		return "Internal Server Error"
	}
	return "Status"
}

// Get sends GET path to host and port and copies the body to body. buf holds
// the request and the response head. It gives the status code.
func Get(s *ip.Stack, host string, port uint16, path string, buf []byte, body io.Writer) (int, error) {
	addr, err := s.LookupHost(host, 5*time.Second)
	if err != nil {
		return 0, err
	}
	c, err := s.DialTCP(addr, port, 0, 10*time.Second)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	req := buf[:0]
	for _, part := range [...]string{"GET ", path, " HTTP/1.1\r\nHost: ", host, "\r\nConnection: close\r\n\r\n"} {
		if len(req)+len(part) > len(buf) {
			return 0, ErrHeadTooLarge
		}
		req = append(req, part...)
	}
	if _, err := c.Write(req); err != nil {
		return 0, err
	}
	head, rest, err := readHead(c, buf)
	if err != nil {
		return 0, err
	}
	line, _ := cutLine(head)
	if len(line) < 12 || line[8] != ' ' {
		return 0, ErrBadHead
	}
	status, ok := atoi(line[9:12])
	if !ok {
		return 0, ErrBadHead
	}
	// The body is the rest of the stream, because of Connection: close.
	if _, err := body.Write(rest); err != nil {
		return status, err
	}
	for {
		n, err := c.Read(buf)
		if n > 0 {
			if _, werr := body.Write(buf[:n]); werr != nil {
				return status, werr
			}
		}
		if err == io.EOF {
			return status, nil
		}
		if err != nil {
			return status, err
		}
	}
}
