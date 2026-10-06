package mqtt

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// pipeConn adapts a net.Conn, whose SetReadDeadline gives an error.
type pipeConn struct{ net.Conn }

func (p pipeConn) SetReadDeadline(t time.Time) { p.Conn.SetReadDeadline(t) }

// broker is the other side of the pipe.
type broker struct {
	t *testing.T
	c net.Conn
}

// read gives the next packet: the first byte and the rest.
func (b *broker) read() (byte, []byte) {
	b.t.Helper()
	b.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var h [1]byte
	if _, err := io.ReadFull(b.c, h[:]); err != nil {
		b.t.Fatalf("broker read: %v", err)
	}
	n, shift := 0, 0
	for {
		var d [1]byte
		io.ReadFull(b.c, d[:])
		n |= int(d[0]&0x7f) << shift
		shift += 7
		if d[0]&0x80 == 0 {
			break
		}
	}
	p := make([]byte, n)
	io.ReadFull(b.c, p)
	return h[0], p
}

func (b *broker) write(p ...byte) {
	b.c.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := b.c.Write(p); err != nil {
		b.t.Fatalf("broker write: %v", err)
	}
}

func newPair(t *testing.T, handler func(*Message)) (*Client, *broker) {
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	return New(pipeConn{a}, make([]byte, 256), make([]byte, 256), handler), &broker{t, b}
}

func TestConnect(t *testing.T) {
	c, br := newPair(t, nil)
	done := make(chan error, 1)
	go func() {
		done <- c.Connect(Config{ClientID: "node1", Username: "u", Password: "pw", KeepAlive: 60 * time.Second})
	}()
	first, p := br.read()
	want := []byte{0, 4, 'M', 'Q', 'T', 'T', 4, 0xc2, 0, 60, 0, 5, 'n', 'o', 'd', 'e', '1', 0, 1, 'u', 0, 2, 'p', 'w'}
	if first != 0x10 || !bytes.Equal(p, want) {
		t.Fatalf("CONNECT %#x %x, want 0x10 %x", first, p, want)
	}
	br.write(0x20, 2, 0, 0)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	c, br = newPair(t, nil)
	go func() { done <- c.Connect(Config{ClientID: "x"}) }()
	br.read()
	br.write(0x20, 2, 0, 5) // not authorized
	if err := <-done; err != ErrRefused {
		t.Fatalf("refused: %v", err)
	}
}

func TestSubscribePublish(t *testing.T) {
	var got []string
	c, br := newPair(t, func(m *Message) { got = append(got, string(m.Topic)+"="+string(m.Payload)) })
	done := make(chan error, 1)
	go func() { done <- c.Subscribe("cmd/#", 1) }()
	first, p := br.read()
	if first != 0x82 || !bytes.Equal(p, []byte{0, 1, 0, 5, 'c', 'm', 'd', '/', '#', 1}) {
		t.Fatalf("SUBSCRIBE %#x %x", first, p)
	}
	br.write(0x90, 3, 0, 1, 1)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// While the QoS 1 publish waits, a message comes first.
	go func() { done <- c.Publish("t/1", []byte("hi"), 1, false) }()
	first, p = br.read()
	if first != 0x32 || !bytes.Equal(p, []byte{0, 3, 't', '/', '1', 0, 2, 'h', 'i'}) {
		t.Fatalf("PUBLISH %#x %x", first, p)
	}
	br.write(0x32, 9, 0, 3, 'c', 'm', 'd', 0, 7, 'o', 'n') // QoS 1, id 7
	if first, p = br.read(); first != 0x40 || !bytes.Equal(p, []byte{0, 7}) {
		t.Fatalf("PUBACK of the client %#x %x", first, p)
	}
	br.write(0x40, 2, 0, 2)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "cmd=on" {
		t.Errorf("messages %q", got)
	}

	// A QoS 0 publish with a payload that needs 2 bytes of length.
	payload := bytes.Repeat([]byte{'x'}, 200)
	go func() { done <- c.Publish("t", payload, 0, true) }()
	if first, p = br.read(); first != 0x31 || len(p) != 203 {
		t.Fatalf("long PUBLISH %#x len %d", first, len(p))
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := c.Publish("t", make([]byte, 300), 0, false); err != ErrBuffer {
		t.Errorf("packet larger than tx: %v", err)
	}
	if err := c.Publish("t", nil, 2, false); err != ErrQoS {
		t.Errorf("QoS 2: %v", err)
	}
}

func TestKeepAlive(t *testing.T) {
	c, br := newPair(t, nil)
	done := make(chan error, 1)
	go func() { done <- c.Connect(Config{ClientID: "k", KeepAlive: time.Second}) }()
	br.read()
	br.write(0x20, 2, 0, 0)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Poll sends PINGREQ at 3/4 of the keep alive time, so 2 in 2 seconds.
	go func() {
		for i := 0; i < 20; i++ {
			if err := c.Poll(100 * time.Millisecond); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	pings := 0
	for {
		br.c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		var h [2]byte
		if _, err := io.ReadFull(br.c, h[:]); err != nil {
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
				if pings < 2 {
					t.Errorf("%d PINGREQ in 2 s, want 2", pings)
				}
				return
			default:
				continue
			}
		}
		if h != [2]byte{0xc0, 0} {
			t.Fatalf("packet %x, want PINGREQ", h)
		}
		pings++
		br.write(0xd0, 0)
	}
}
