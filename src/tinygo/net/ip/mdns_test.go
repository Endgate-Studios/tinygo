package ip

import (
	"bytes"
	"testing"
)

var mdnsMAC = [6]byte{0x01, 0x00, 0x5e, 0x00, 0x00, 0xfb}

func TestMulticast(t *testing.T) {
	s, l, _ := newTestStack(t)
	c, err := s.ListenUDP(5000, 256)
	if err != nil {
		t.Fatal(err)
	}
	group := Addr{224, 0, 0, 200}
	frame := ethernet(multicastMAC(group), peerMAC, etherIPv4, udpPacket(peerIP, group, 5000, 5000, []byte("g")))
	s.Input(frame)
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 8)); ok {
		t.Fatal("a datagram of a group that is not joined came in")
	}
	if err := s.JoinMulticast(group); err != nil {
		t.Fatal(err)
	}
	s.Input(frame)
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 8)); !ok {
		t.Fatal("no datagram of the joined group")
	}
	if err := c.WriteTo([]byte("x"), group, 5000); err != nil {
		t.Fatal(err)
	}
	f := l.next(t)
	if [6]byte(f[0:6]) != multicastMAC(group) || f[14+8] != 255 {
		t.Fatalf("frame to the group: MAC %x TTL %d", f[0:6], f[14+8])
	}
	s.LeaveMulticast(group)
	s.Input(frame)
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 8)); ok {
		t.Fatal("a datagram came in after LeaveMulticast")
	}
	if s.JoinMulticast(Addr{192, 168, 1, 1}) != ErrNotMulticast {
		t.Error("JoinMulticast took a unicast address")
	}
}

// mdnsQuery makes a query for name with type typ, as the first question.
func mdnsQuery(id uint16, name string, typ uint16) []byte {
	b := make([]byte, 128)
	n, _ := putQuery(b, name)
	b[2] = 0 // no RD
	put16(b[0:], id)
	put16(b[n-4:], typ)
	return b[:n]
}

// mdnsAddr gives the address of the A record in a response.
func mdnsAddr(t *testing.T, f []byte) (Addr, uint16) {
	t.Helper()
	m := f[14+20+8:]
	off := dnsHdrLen
	var ok bool
	for q := be16(m[4:]); q > 0; q-- {
		if off, ok = skipName(m, off); !ok {
			t.Fatalf("response %x: bad question", m)
		}
		off += 4
	}
	if off, ok = skipName(m, off); !ok || be16(m[6:]) != 1 || off+14 > len(m) {
		t.Fatalf("response %x: no answer", m)
	}
	if be16(m[off:]) != dnsTypeA || be16(m[off+2:])&^mdnsCacheFlush != dnsClassIN || be16(m[off+8:]) != 4 {
		t.Fatalf("response %x: not an A record", m)
	}
	return Addr(m[off+10 : off+14]), be16(m[0:])
}

func TestMDNS(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	go s.ServeMDNS("tinygo-node")

	// Two announcements, one second apart.
	for i := 0; i < mdnsAnnounces; i++ {
		f := l.next(t)
		if [6]byte(f[0:6]) != mdnsMAC {
			t.Fatalf("announcement %d to %x", i, f[0:6])
		}
		if a, _ := mdnsAddr(t, f); a != myIP {
			t.Fatalf("announcement %d gives %v", i, a)
		}
	}

	query := func(sport uint16, q []byte) {
		s.Input(ethernet(mdnsMAC, peerMAC, etherIPv4, udpPacket(peerIP, MDNSGroup, sport, mdnsPort, q)))
	}
	query(mdnsPort, mdnsQuery(0, "TinyGo-Node.local", dnsTypeA))
	f := l.next(t)
	if a, id := mdnsAddr(t, f); [6]byte(f[0:6]) != mdnsMAC || a != myIP || id != 0 {
		t.Fatalf("multicast answer to %x: %v id %d", f[0:6], a, id)
	}

	// Another name or another type gets no answer.
	query(mdnsPort, mdnsQuery(0, "other.local", dnsTypeA))
	query(mdnsPort, mdnsQuery(0, "tinygo-node.local", 28)) // AAAA
	noSeg(t, l)

	// A legacy resolver on another port gets a unicast answer with its ID.
	query(40000, mdnsQuery(0x1234, "tinygo-node.local", dnsTypeA))
	f = l.next(t)
	a, id := mdnsAddr(t, f)
	if [6]byte(f[0:6]) != peerMAC || a != myIP || id != 0x1234 || be16(f[14+20+2:]) != 40000 {
		t.Fatalf("legacy answer to %x port %d: %v id %#x", f[0:6], be16(f[14+20+2:]), a, id)
	}
	if !bytes.Contains(f, []byte("\x0btinygo-node\x05local")) {
		t.Error("the legacy answer has no question")
	}
}

func TestMDNSCompressedQuestion(t *testing.T) {
	// The second question points to the name of the first one.
	q := mdnsQuery(0, "other.local", dnsTypeA)
	put16(q[4:], 2)
	q = append(q, 0x0b)
	q = append(q, "tinygo-node"...)
	q = append(q, 0xc0, dnsHdrLen+6, 0, dnsTypeA, 0x80, dnsClassIN) // .local, QU bit
	if _, ok := mdnsQuestion(q, "tinygo-node"); !ok {
		t.Fatal("no match for a compressed name")
	}
	if _, ok := mdnsQuestion(q[:len(q)-3], "tinygo-node"); ok {
		t.Fatal("a short message matched")
	}
	if err := (&Stack{}).ServeMDNS("a.b"); err != ErrBadName {
		t.Errorf("name with a dot: %v", err)
	}
}
