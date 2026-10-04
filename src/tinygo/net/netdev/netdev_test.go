package netdev

import (
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"runtime/memory/arena"
	"tinygo/net/ip"
)

// wire is one direction of a cable between two stacks. Frames go to the
// other stack from a goroutine, because a stack holds its lock in Send.
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
	mask  = ip.Addr{255, 255, 255, 0}
)

// pair makes stack A, used through net, and stack B, the peer.
func pair(t *testing.T) (a, b *ip.Stack, memA *arena.Heap) {
	t.Helper()
	wa := &wire{mac: [6]byte{2, 0, 0, 0, 0, 1}, out: make(chan []byte, 64)}
	wb := &wire{mac: [6]byte{2, 0, 0, 0, 0, 2}, out: make(chan []byte, 64)}
	memA, _ = arena.NewHeap(make([]byte, 32<<10))
	memB, _ := arena.NewHeap(make([]byte, 32<<10))
	a, b = ip.New(wa, memA), ip.New(wb, memB)
	a.SetConfig(ip.Config{Addr: addrA, Mask: mask})
	b.SetConfig(ip.Config{Addr: addrB, Mask: mask})
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
	New(a, 1024).Use()
	return a, b, memA
}

func tcpEcho(t *testing.T, b *ip.Stack, port uint16) {
	l, err := b.ListenTCP(port, 1024)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 256)
		for {
			n, err := c.Read(buf)
			if err != nil {
				break
			}
			c.Write(buf[:n])
		}
		c.Close()
	}()
}

func TestDialTCPAndDeadline(t *testing.T) {
	_, b, memA := pair(t)
	tcpEcho(t, b, 7)
	c, err := net.Dial("tcp", "10.0.0.2:7")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("ping over net")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, err := io.ReadAtLeast(c, buf, len("ping over net"))
	if err != nil || string(buf[:n]) != "ping over net" {
		t.Fatalf("echo %q %v", buf[:n], err)
	}
	c.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	start := time.Now()
	if _, err := c.Read(buf); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Read after the deadline: %v", err)
	}
	if d := time.Since(start); d < 90*time.Millisecond || d > time.Second {
		t.Errorf("the deadline took %v", d)
	}
	c.Close()
	time.Sleep(2500 * time.Millisecond) // TIME-WAIT of the side that closed first
	if in := memA.Stats().InUse; in != 0 {
		t.Errorf("rings of A not freed: %d bytes", in)
	}
}

func TestListenTCP(t *testing.T) {
	_, b, _ := pair(t)
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := b.DialTCP(addrA, 8080, 1024, 3*time.Second)
		if err != nil {
			return
		}
		c.Write([]byte("hello A"))
		buf := make([]byte, 16)
		n, _ := c.Read(buf)
		c.Write(append([]byte("got "), buf[:n]...))
		c.Close()
	}()
	c, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if ra := c.RemoteAddr().String(); ra[:9] != "10.0.0.2:" {
		t.Errorf("remote %s", ra)
	}
	buf := make([]byte, 64)
	n, _ := io.ReadAtLeast(c, buf, 7)
	if string(buf[:n]) != "hello A" {
		t.Fatalf("got %q", buf[:n])
	}
	c.Write([]byte("hi B"))
	all, err := io.ReadAll(c)
	if err != nil || string(all) != "got hi B" {
		t.Fatalf("ReadAll %q %v", all, err)
	}
	c.Close()
}

func TestDialUDP(t *testing.T) {
	_, b, _ := pair(t)
	u, err := b.ListenUDP(9, 1024)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 256)
		n, from, port, err := u.ReadFrom(buf)
		if err == nil {
			u.WriteTo(buf[:n], from, port)
		}
	}()
	c, err := net.Dial("udp", "10.0.0.2:9")
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("datagram"))
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil || string(buf[:n]) != "datagram" {
		t.Fatalf("got %q %v", buf[:n], err)
	}
	c.Close()
}

func TestLookupLiteral(t *testing.T) {
	pair(t)
	addrs, err := net.LookupHost("10.0.0.2")
	if err != nil || len(addrs) == 0 || addrs[0] != "10.0.0.2" {
		t.Fatalf("got %v %v", addrs, err)
	}
}
