package ip

import (
	"testing"
	"time"

	"runtime/memory/arena"
)

// pipeLink sends each frame to the Input of the other stack, from a
// goroutine, because a stack holds its lock in Send.
type pipeLink struct {
	mac  [6]byte
	sent chan []byte
}

func (l *pipeLink) MAC() [6]byte { return l.mac }

func (l *pipeLink) Send(f []byte) error {
	l.sent <- append([]byte(nil), f...)
	return nil
}

func pipe(from *pipeLink, to *Stack, done <-chan struct{}) {
	for {
		select {
		case f := <-from.sent:
			to.Input(f)
		case <-done:
			return
		}
	}
}

func newPipeStack(t *testing.T, mac [6]byte) (*Stack, *pipeLink) {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 16<<10))
	if err != nil {
		t.Fatal(err)
	}
	l := &pipeLink{mac: mac, sent: make(chan []byte, 16)}
	return New(l, h), l
}

// TestDHCPServerWithClient gives an address to the DHCP client of this
// package.
func TestDHCPServerWithClient(t *testing.T) {
	ap, apLink := newPipeStack(t, [6]byte{0x02, 0, 0, 0, 0, 0xaa})
	ap.SetConfig(Config{Addr: Addr{192, 168, 4, 1}, Mask: mask24})
	sta, staLink := newPipeStack(t, [6]byte{0x02, 0, 0, 0, 0, 0x01})
	done := make(chan struct{})
	defer close(done)
	go pipe(apLink, sta, done)
	go pipe(staLink, ap, done)
	go ap.ServeDHCP(Addr{192, 168, 4, 2}, 4, time.Hour)

	if err := sta.DHCP(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	want := Config{Addr: Addr{192, 168, 4, 2}, Mask: mask24, Gateway: Addr{192, 168, 4, 1}, DNS: Addr{192, 168, 4, 1}}
	if c := sta.Config(); c != want {
		t.Errorf("config %+v, want %+v", c, want)
	}
	if sta.Lease() != time.Hour {
		t.Errorf("lease %v, want 1h", sta.Lease())
	}
}

// request makes a client message of type msg from mac, with the options of
// requested and server when they are not zero.
func request(msg byte, mac [6]byte, requested, server Addr) []byte {
	p := make([]byte, dhcpFixedLen)
	p[0], p[1], p[2] = 1, 1, 6
	put32(p[4:], 0x1234)
	copy(p[28:34], mac[:])
	put32(p[236:], 0x63825363)
	p = append(p, optMessageType, 1, msg)
	if !requested.IsZero() {
		p = append(p, optRequestedIP, 4, requested[0], requested[1], requested[2], requested[3])
	}
	if !server.IsZero() {
		p = append(p, optServerID, 4, server[0], server[1], server[2], server[3])
	}
	return append(p, optEnd)
}

// replyOf gives the yiaddr and the message type of a reply.
func replyOf(t *testing.T, b []byte) (Addr, byte) {
	t.Helper()
	if len(b) < dhcpFixedLen+3 || b[0] != 2 || be32(b[236:]) != 0x63825363 {
		t.Fatalf("bad reply %x", b)
	}
	return Addr(b[16:20]), b[dhcpFixedLen+2]
}

func TestDHCPServerMessages(t *testing.T) {
	s, _, _ := newTestStack(t)
	me := s.Config().Addr
	d := &dhcpPool{first: Addr{192, 168, 1, 100}, n: 2, lease: time.Minute}
	macA, macB, macC := [6]byte{2, 0, 0, 0, 0, 0xa}, [6]byte{2, 0, 0, 0, 0, 0xb}, [6]byte{2, 0, 0, 0, 0, 0xc}

	n := d.handle(s, request(dhcpDiscover, macA, Addr{}, Addr{}))
	if a, msg := replyOf(t, d.out[:n]); msg != dhcpOffer || a != (Addr{192, 168, 1, 100}) {
		t.Errorf("offer for A: %v type %d", a, msg)
	}
	n = d.handle(s, request(dhcpRequest, macA, Addr{192, 168, 1, 100}, me))
	if a, msg := replyOf(t, d.out[:n]); msg != dhcpAck || a != (Addr{192, 168, 1, 100}) {
		t.Errorf("ack for A: %v type %d", a, msg)
	}
	// A asks for an address that it does not have.
	n = d.handle(s, request(dhcpRequest, macA, Addr{192, 168, 1, 101}, me))
	if _, msg := replyOf(t, d.out[:n]); msg != dhcpNak {
		t.Errorf("request of another address: type %d, want NAK", msg)
	}
	// B gets the second address. Then the pool is full for C.
	n = d.handle(s, request(dhcpDiscover, macB, Addr{}, Addr{}))
	if a, _ := replyOf(t, d.out[:n]); a != (Addr{192, 168, 1, 101}) {
		t.Errorf("offer for B: %v", a)
	}
	if n = d.handle(s, request(dhcpDiscover, macC, Addr{}, Addr{})); n != 0 {
		t.Errorf("full pool gave an offer to C")
	}
	// A request for another server frees the address of B.
	if n = d.handle(s, request(dhcpRequest, macB, Addr{192, 168, 1, 101}, Addr{10, 0, 0, 1})); n != 0 {
		t.Errorf("request for another server got a reply")
	}
	n = d.handle(s, request(dhcpDiscover, macC, Addr{}, Addr{}))
	if a, _ := replyOf(t, d.out[:n]); a != (Addr{192, 168, 1, 101}) {
		t.Errorf("offer for C after B left: %v", a)
	}
	// RELEASE frees the address of A.
	d.handle(s, request(dhcpRelease, macA, Addr{}, Addr{}))
	if d.find(macA) >= 0 {
		t.Error("A keeps its address after RELEASE")
	}
	// A message that is not a client request gets no reply.
	bad := request(dhcpDiscover, macA, Addr{}, Addr{})
	bad[0] = 2
	if n = d.handle(s, bad); n != 0 {
		t.Error("a BOOTREPLY got a reply")
	}
}

func TestDHCPServerPool(t *testing.T) {
	s, _, _ := newTestStack(t)
	for _, n := range []int{0, MaxDHCPClients + 1} {
		if err := s.ServeDHCP(Addr{192, 168, 1, 100}, n, time.Minute); err != ErrDHCPPool {
			t.Errorf("pool of %d: %v, want ErrDHCPPool", n, err)
		}
	}
	if err := s.ServeDHCP(Addr{192, 168, 1, 250}, 8, time.Minute); err != ErrDHCPPool {
		t.Errorf("pool past .255: %v, want ErrDHCPPool", err)
	}
}
