package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"runtime/memory/arena"
	"tinygo/net/ip"
)

type wire struct {
	mac [6]byte
	out chan []byte
}

func (w *wire) MAC() [6]byte { return w.mac }

func (w *wire) Send(f []byte) error {
	w.out <- append([]byte(nil), f...)
	return nil
}

var (
	addrA = ip.Addr{10, 0, 0, 1}
	addrB = ip.Addr{10, 0, 0, 2}
)

func pair(t *testing.T) (a, b *ip.Stack) {
	t.Helper()
	wa := &wire{mac: [6]byte{2, 0, 0, 0, 0, 1}, out: make(chan []byte, 64)}
	wb := &wire{mac: [6]byte{2, 0, 0, 0, 0, 2}, out: make(chan []byte, 64)}
	ma, _ := arena.NewHeap(make([]byte, 32<<10))
	mb, _ := arena.NewHeap(make([]byte, 32<<10))
	a, b = ip.New(wa, ma), ip.New(wb, mb)
	a.SetConfig(ip.Config{Addr: addrA, Mask: ip.Addr{255, 255, 255, 0}})
	b.SetConfig(ip.Config{Addr: addrB, Mask: ip.Addr{255, 255, 255, 0}})
	go func() {
		for f := range wa.out {
			b.Input(f)
		}
	}()
	go func() {
		for f := range wb.out {
			a.Input(f)
		}
	}()
	return a, b
}

var big = strings.Repeat("0123456789", 300) // larger than Response.out

func handler(w *Response, r *Request) {
	switch string(r.Path) {
	case "/hello":
		w.WriteString("method=")
		w.Write(r.Method)
		w.WriteString(" query=")
		w.Write(r.Query)
		w.WriteString(" agent=")
		w.Write(r.Header("x-AGENT"))
	case "/post":
		var b [64]byte
		n := 0
		for {
			m, err := r.Read(b[n:])
			n += m
			if err != nil || n == len(b) {
				break
			}
		}
		w.WriteHeader(200, "application/octet-stream")
		w.Write(b[:n])
	case "/big":
		w.WriteString(big)
	default:
		w.WriteHeader(404, "text/plain")
		w.WriteString("not found")
	}
}

// rawRequest sends text on a new connection from b and gives the whole answer.
func rawRequest(t *testing.T, b *ip.Stack, parts ...string) string {
	t.Helper()
	c, err := b.DialTCP(addrA, 80, 4096, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, p := range parts {
		c.Write([]byte(p))
		time.Sleep(20 * time.Millisecond)
	}
	var out bytes.Buffer
	buf := make([]byte, 512)
	for {
		n, err := c.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			return out.String()
		}
	}
}

func TestServerAndGet(t *testing.T) {
	a, b := pair(t)
	l, err := a.ListenTCP(80, 2048)
	if err != nil {
		t.Fatal(err)
	}
	go NewServer(l, make([]byte, 512), handler).Serve()

	var body bytes.Buffer
	status, err := Get(b, "10.0.0.1", 80, "/hello?x=1", make([]byte, 512), &body)
	if err != nil || status != 200 {
		t.Fatalf("Get: %d %v", status, err)
	}
	// Get sends no X-Agent header.
	if got := body.String(); got != "method=GET query=x=1 agent=" {
		t.Errorf("body %q", got)
	}

	got := rawRequest(t, b, "GET /hello HTTP/1.1\r\nX-Agent:  tiny \r\n\r\n")
	if !strings.HasPrefix(got, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n") || !strings.HasSuffix(got, "agent=tiny") {
		t.Errorf("answer %q", got)
	}

	// The body comes partly with the head and partly after it.
	got = rawRequest(t, b, "POST /post HTTP/1.1\r\nContent-Length: 11\r\n\r\nhello", " world")
	if !strings.HasSuffix(got, "\r\n\r\nhello world") {
		t.Errorf("POST answer %q", got)
	}

	if got := rawRequest(t, b, "GET /nothing HTTP/1.1\r\n\r\n"); !strings.HasPrefix(got, "HTTP/1.1 404 Not Found") {
		t.Errorf("404 answer %q", got)
	}
	long := "GET / HTTP/1.1\r\nX-Long: " + strings.Repeat("a", 600) + "\r\n\r\n"
	if got := rawRequest(t, b, long); !strings.HasPrefix(got, "HTTP/1.1 431 ") {
		t.Errorf("431 answer %q", got)
	}

	body.Reset()
	status, err = Get(b, "10.0.0.1", 80, "/big", make([]byte, 512), &body)
	if err != nil || status != 200 || body.String() != big {
		t.Fatalf("big: %d %v, %d bytes", status, err, body.Len())
	}
}

func TestFindHeader(t *testing.T) {
	h := []byte("Host: a\r\nContent-Length:\t42 \r\nX: y")
	if v := findHeader(h, "content-length"); string(v) != "42" {
		t.Errorf("got %q", v)
	}
	if v := findHeader(h, "missing"); v != nil {
		t.Errorf("got %q", v)
	}
}
