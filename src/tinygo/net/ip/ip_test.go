package ip

import (
	"bytes"
	"testing"
	"time"

	"runtime/memory/arena"
)

var (
	myMAC   = [6]byte{0x02, 0, 0, 0, 0, 0x01}
	peerMAC = [6]byte{0x02, 0, 0, 0, 0, 0x02}
	myIP    = Addr{192, 168, 1, 10}
	peerIP  = Addr{192, 168, 1, 20}
	gwIP    = Addr{192, 168, 1, 1}
	mask24  = Addr{255, 255, 255, 0}
)

// fakeLink puts sent frames on a channel. A test goroutine answers them with
// Input, never from Send, because the stack holds its lock in Send.
type fakeLink struct{ sent chan []byte }

func (l *fakeLink) MAC() [6]byte { return myMAC }

func (l *fakeLink) Send(f []byte) error {
	l.sent <- append([]byte(nil), f...)
	return nil
}

func newTestStack(t *testing.T) (*Stack, *fakeLink, *arena.Heap) {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 16<<10))
	if err != nil {
		t.Fatal(err)
	}
	l := &fakeLink{sent: make(chan []byte, 16)}
	s := New(l, h)
	s.SetConfig(Config{Addr: myIP, Mask: mask24, Gateway: gwIP})
	return s, l, h
}

func (l *fakeLink) next(t *testing.T) []byte {
	t.Helper()
	select {
	case f := <-l.sent:
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no frame sent")
		return nil
	}
}

// refChecksum is RFC 1071, written apart from the stack code.
func refChecksum(parts ...[]byte) uint16 {
	var all []byte
	for _, p := range parts {
		all = append(all, p...)
	}
	if len(all)%2 == 1 {
		all = append(all, 0)
	}
	var sum uint32
	for i := 0; i < len(all); i += 2 {
		sum += uint32(all[i])<<8 | uint32(all[i+1])
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func ethernet(dst, src [6]byte, typ uint16, payload []byte) []byte {
	f := append(append(append([]byte(nil), dst[:]...), src[:]...), byte(typ>>8), byte(typ))
	return append(f, payload...)
}

func arpPacket(op uint16, sha [6]byte, spa Addr, tha [6]byte, tpa Addr) []byte {
	p := []byte{0, 1, 0x08, 0x00, 6, 4, byte(op >> 8), byte(op)}
	p = append(p, sha[:]...)
	p = append(p, spa[:]...)
	p = append(p, tha[:]...)
	return append(p, tpa[:]...)
}

func ipv4Packet(src, dst Addr, proto byte, payload []byte) []byte {
	h := []byte{0x45, 0, 0, 0, 0, 1, 0, 0, 64, proto, 0, 0}
	h = append(append(h, src[:]...), dst[:]...)
	total := len(h) + len(payload)
	h[2], h[3] = byte(total>>8), byte(total)
	c := refChecksum(h)
	h[10], h[11] = byte(c>>8), byte(c)
	return append(h, payload...)
}

func udpPacket(src, dst Addr, sport, dport uint16, data []byte) []byte {
	u := []byte{byte(sport >> 8), byte(sport), byte(dport >> 8), byte(dport), 0, 0, 0, 0}
	n := len(u) + len(data)
	u[4], u[5] = byte(n>>8), byte(n)
	u = append(u, data...)
	pseudo := append(append(append([]byte(nil), src[:]...), dst[:]...), 0, 17, byte(n>>8), byte(n))
	c := refChecksum(pseudo, u)
	u[6], u[7] = byte(c>>8), byte(c)
	return ipv4Packet(src, dst, 17, u)
}

func TestARPReply(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.Input(ethernet(broadcastMAC, peerMAC, etherARP, arpPacket(1, peerMAC, peerIP, [6]byte{}, myIP)))
	f := l.next(t)
	want := ethernet(peerMAC, myMAC, etherARP, arpPacket(2, myMAC, myIP, peerMAC, peerIP))
	if !bytes.Equal(f, want) {
		t.Fatalf("ARP reply\n got %x\nwant %x", f, want)
	}
	if mac, ok := s.arp.lookup(peerIP); !ok || mac != peerMAC {
		t.Error("the requester is not in the ARP cache")
	}
	// A request for another host is not answered.
	s.Input(ethernet(broadcastMAC, peerMAC, etherARP, arpPacket(1, peerMAC, peerIP, [6]byte{}, gwIP)))
	select {
	case f := <-l.sent:
		t.Errorf("answered a request for another host: %x", f)
	default:
	}
}

func TestEchoReply(t *testing.T) {
	s, l, _ := newTestStack(t)
	icmp := []byte{8, 0, 0, 0, 0x12, 0x34, 0, 1, 'p', 'i', 'n', 'g'}
	c := refChecksum(icmp)
	icmp[2], icmp[3] = byte(c>>8), byte(c)
	s.Input(ethernet(myMAC, peerMAC, etherIPv4, ipv4Packet(peerIP, myIP, 1, icmp)))
	f := l.next(t)
	if [6]byte(f[0:6]) != peerMAC || be16(f[12:]) != etherIPv4 {
		t.Fatalf("bad Ethernet header %x", f[:14])
	}
	ip := f[14:]
	if refChecksum(ip[:20]) != 0 || Addr(ip[12:16]) != myIP || Addr(ip[16:20]) != peerIP {
		t.Fatalf("bad IP header %x", ip[:20])
	}
	reply := ip[20:]
	if reply[0] != 0 || refChecksum(reply) != 0 || !bytes.Equal(reply[4:], icmp[4:]) {
		t.Fatalf("bad echo reply %x", reply)
	}
	if s.Stats().EchoReplies != 1 {
		t.Error("EchoReplies not counted")
	}
}

func TestUDPReceiveAndSend(t *testing.T) {
	s, l, h := newTestStack(t)
	c, err := s.ListenUDP(7, 256)
	if err != nil {
		t.Fatal(err)
	}
	s.Input(ethernet(myMAC, peerMAC, etherIPv4, udpPacket(peerIP, myIP, 5000, 7, []byte("hello"))))
	buf := make([]byte, 64)
	n, from, port, err := c.ReadFrom(buf)
	if err != nil || string(buf[:n]) != "hello" || from != peerIP || port != 5000 {
		t.Fatalf("ReadFrom = %q %v %d %v", buf[:n], from, port, err)
	}

	// The peer is not in the ARP cache: WriteTo asks for it first.
	done := make(chan error, 1)
	go func() { done <- c.WriteTo([]byte("echo"), peerIP, 5000) }()
	req := l.next(t)
	if be16(req[12:]) != etherARP || Addr(req[14+24:14+28]) != peerIP {
		t.Fatalf("expected an ARP request for the peer, got %x", req)
	}
	s.Input(ethernet(myMAC, peerMAC, etherARP, arpPacket(2, peerMAC, peerIP, myMAC, myIP)))
	f := l.next(t)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	want := ethernet(peerMAC, myMAC, etherIPv4, udpPacket(myIP, peerIP, 7, 5000, []byte("echo")))
	if !bytes.Equal(f[:14], want[:14]) || !bytes.Equal(f[14+20:], want[14+20:]) || refChecksum(f[14:34]) != 0 {
		t.Fatalf("UDP frame\n got %x\nwant %x", f, want)
	}

	c.Close()
	if _, _, _, err := c.ReadFrom(buf); err != ErrClosed {
		t.Errorf("ReadFrom after Close = %v", err)
	}
	if h.Stats().InUse != 0 {
		t.Errorf("socket buffer not freed: %d bytes in use", h.Stats().InUse)
	}
}

// TestUDPSubnetBroadcast sends to the subnet broadcast address with no ARP,
// and receives a datagram to it.
func TestUDPSubnetBroadcast(t *testing.T) {
	s, l, _ := newTestStack(t)
	c, err := s.ListenUDP(4210, 256)
	if err != nil {
		t.Fatal(err)
	}
	bcast := Addr{192, 168, 1, 255}
	if err := c.WriteTo([]byte("node"), bcast, 4210); err != nil {
		t.Fatal(err)
	}
	f := l.next(t)
	want := ethernet(broadcastMAC, myMAC, etherIPv4, udpPacket(myIP, bcast, 4210, 4210, []byte("node")))
	if !bytes.Equal(f[:14], want[:14]) || !bytes.Equal(f[14+20:], want[14+20:]) {
		t.Fatalf("broadcast frame\n got %x\nwant %x", f, want)
	}
	noSeg(t, l) // one frame for one WriteTo
	for _, dst := range []Addr{bcast, Broadcast} {
		s.Input(ethernet(broadcastMAC, peerMAC, etherIPv4, udpPacket(peerIP, dst, 4210, 4210, []byte("peer"))))
		buf := make([]byte, 16)
		n, from, _, ok, err := c.TryReadFrom(buf)
		if !ok || err != nil || string(buf[:n]) != "peer" || from != peerIP {
			t.Fatalf("datagram to %v: %q from %v ok %v %v", dst, buf[:n], from, ok, err)
		}
	}
	// A broadcast of another subnet is not for this stack.
	s.Input(ethernet(broadcastMAC, peerMAC, etherIPv4, udpPacket(peerIP, Addr{10, 0, 0, 255}, 4210, 4210, []byte("other"))))
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 16)); ok {
		t.Error("a broadcast of another subnet came in")
	}
}

func TestUDPRingWrapAndDrop(t *testing.T) {
	s, _, _ := newTestStack(t)
	c, err := s.ListenUDP(9, 64)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	for round := 0; round < 20; round++ {
		// Two records of 8+20 bytes fit in 64 bytes, a third does not.
		for i := 0; i < 3; i++ {
			msg := bytes.Repeat([]byte{byte('a' + round%26), byte(i)}, 10)
			s.Input(ethernet(myMAC, peerMAC, etherIPv4, udpPacket(peerIP, myIP, 1, 9, msg)))
		}
		for i := 0; i < 2; i++ {
			n, _, _, ok, err := c.TryReadFrom(buf)
			want := bytes.Repeat([]byte{byte('a' + round%26), byte(i)}, 10)
			if !ok || err != nil || !bytes.Equal(buf[:n], want) {
				t.Fatalf("round %d record %d: %q ok %v %v", round, i, buf[:n], ok, err)
			}
		}
		if _, _, _, ok, _ := c.TryReadFrom(buf); ok {
			t.Fatalf("round %d: a third record fit", round)
		}
	}
	if c.Dropped != 20 {
		t.Errorf("Dropped = %d, want 20", c.Dropped)
	}
}

func TestListenUDPErrors(t *testing.T) {
	s, _, _ := newTestStack(t)
	if _, err := s.ListenUDP(5, 64); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListenUDP(5, 64); err != ErrPortInUse {
		t.Errorf("same port: %v", err)
	}
	c, err := s.ListenUDP(0, 64)
	if err != nil || c.Port() < ephemeralBase {
		t.Errorf("ephemeral port %d, %v", c.Port(), err)
	}
	if _, err := s.ListenUDP(6, 1<<20); err != ErrNoMemory {
		t.Errorf("huge buffer: %v", err)
	}
}

// dhcpServer answers the DISCOVER and the REQUEST of the client.
func dhcpServer(t *testing.T, s *Stack, l *fakeLink, nak bool) {
	dhcpServerLease(t, s, l, nak, 3600, nil)
}

// dhcpServerLease answers DHCP messages with a lease of secs seconds until an
// ACK. renew, when not nil, receives the REQUEST that the ACK answers.
func dhcpServerLease(t *testing.T, s *Stack, l *fakeLink, nak bool, secs uint32, renew chan<- []byte) {
	server := Addr{192, 168, 1, 1}
	offer := Addr{192, 168, 1, 77}
	for {
		f := <-l.sent
		if be16(f[12:]) != etherIPv4 || f[14+9] != 17 || be16(f[14+20+2:]) != dhcpServerPort {
			continue
		}
		req := f[14+20+8:]
		var msg byte
		for o := req[dhcpFixedLen:]; len(o) > 1 && o[0] != optEnd; o = o[2+int(o[1]):] {
			if o[0] == optMessageType {
				msg = o[2]
			}
		}
		reply := make([]byte, dhcpFixedLen)
		reply[0], reply[1], reply[2] = 2, 1, 6
		copy(reply[4:8], req[4:8]) // xid
		copy(reply[16:20], offer[:])
		copy(reply[28:34], req[28:34])
		put32(reply[236:], 0x63825363)
		typ := byte(dhcpOffer)
		if msg == dhcpRequest {
			typ = dhcpAck
			if nak {
				typ, nak = dhcpNak, false
			}
		}
		reply = append(reply, optMessageType, 1, typ,
			optServerID, 4, server[0], server[1], server[2], server[3],
			optSubnetMask, 4, 255, 255, 255, 0,
			optRouter, 4, 192, 168, 1, 1,
			optDNS, 4, 8, 8, 8, 8,
			optLeaseTime, 4, byte(secs>>24), byte(secs>>16), byte(secs>>8), byte(secs),
			optEnd)
		s.Input(ethernet(broadcastMAC, peerMAC, etherIPv4, udpPacket(server, Broadcast, dhcpServerPort, dhcpClientPort, reply)))
		if typ == dhcpAck {
			if renew != nil {
				renew <- f
			}
			return
		}
	}
}

func TestDHCP(t *testing.T) {
	for _, nak := range []bool{false, true} {
		s, l, _ := newTestStack(t)
		go dhcpServer(t, s, l, nak)
		if err := s.DHCP(20 * time.Second); err != nil {
			t.Fatalf("nak %v: %v", nak, err)
		}
		want := Config{Addr: Addr{192, 168, 1, 77}, Mask: mask24, Gateway: Addr{192, 168, 1, 1}, DNS: Addr{8, 8, 8, 8}}
		if c := s.Config(); c != want {
			t.Errorf("nak %v: config %+v, want %+v", nak, c, want)
		}
		if s.Lease() != time.Hour {
			t.Errorf("lease %v", s.Lease())
		}
	}
}

func TestAddrString(t *testing.T) {
	if got := (Addr{192, 168, 0, 7}).String(); got != "192.168.0.7" {
		t.Errorf("String = %q", got)
	}
}

func TestDHCPRenew(t *testing.T) {
	s, l, _ := newTestStack(t)
	server := Addr{192, 168, 1, 1}
	s.arp.update(server, peerMAC)
	go dhcpServerLease(t, s, l, false, 4, nil)
	if err := s.DHCP(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	first := s.dhcp.start
	s.mu.Unlock()

	// At T1 (2 s) the client unicasts a REQUEST with ciaddr and no option 50.
	got := make(chan []byte, 1)
	go dhcpServerLease(t, s, l, false, 4, got)
	var req []byte
	select {
	case req = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no renewal")
	}
	ip := req[14:]
	dh := ip[20+8:]
	if Addr(ip[16:20]) != server || Addr(ip[12:16]) != (Addr{192, 168, 1, 77}) {
		t.Errorf("REQUEST from %v to %v, want a unicast to the server", Addr(ip[12:16]), Addr(ip[16:20]))
	}
	if Addr(dh[12:16]) != (Addr{192, 168, 1, 77}) || bytes.Contains(dh[dhcpFixedLen:], []byte{optRequestedIP, 4}) {
		t.Errorf("RENEWING needs ciaddr and no requested address: %x", dh[12:16])
	}
	time.Sleep(100 * time.Millisecond)
	s.mu.Lock()
	renewed := s.dhcp.start.After(first)
	s.mu.Unlock()
	if !renewed || s.Config().Addr != (Addr{192, 168, 1, 77}) {
		t.Error("the lease did not start again")
	}
}
