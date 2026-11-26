package ip

import (
	"bytes"
	"testing"
	"time"
)

func TestDNSQuery(t *testing.T) {
	var b [300]byte
	n, err := putQuery(b[:], "a.bc.")
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 'a', 2, 'b', 'c', 0, 0, 1, 0, 1}
	if !bytes.Equal(b[:n], want) {
		t.Fatalf("query %x, want %x", b[:n], want)
	}
	for _, bad := range []string{"", ".", "a..b", string(make([]byte, 64)) + ".x"} {
		if _, err := putQuery(b[:], bad); err != ErrBadName {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// dnsReply answers the query q with a CNAME to "x.y" and an A record for it,
// both with compression pointers.
func dnsReply(q []byte, rcode byte, a Addr) []byte {
	m := append([]byte(nil), q...)
	m[2], m[3] = 0x81, 0x80|rcode
	if rcode != 0 {
		return m
	}
	m[7] = 2 // two answers
	// The CNAME: name is a pointer to the question name at offset 12.
	m = append(m, 0xc0, 12, 0, 5, 0, 1, 0, 0, 0, 60, 0, 5, 1, 'x', 1, 'y', 0)
	cname := len(m) - 5
	m = append(m, 0xc0, byte(cname), 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, a[0], a[1], a[2], a[3])
	return m
}

func TestDNSAnswer(t *testing.T) {
	var q [300]byte
	n, _ := putQuery(q[:], "www.example.com")
	want := Addr{93, 184, 216, 34}
	if got, err := parseAnswer(dnsReply(q[:n], 0, want)); err != nil || got != want {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := parseAnswer(dnsReply(q[:n], 3, want)); err != ErrNotFound {
		t.Errorf("NXDOMAIN: %v", err)
	}
	cut := dnsReply(q[:n], 0, want)
	if _, err := parseAnswer(cut[:len(cut)-3]); err != ErrDNSFormat {
		t.Errorf("cut answer: %v", err)
	}
}

func TestParseAddr(t *testing.T) {
	if a, ok := parseAddr("10.0.255.1"); !ok || a != (Addr{10, 0, 255, 1}) {
		t.Errorf("got %v %v", a, ok)
	}
	for _, bad := range []string{"1.2.3", "1.2.3.4.5", "1.2.3.256", "a.b.c.d", "1..2.3"} {
		if _, ok := parseAddr(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestLookupHost(t *testing.T) {
	s, l, h := newTestStack(t)
	s.SetConfig(Config{Addr: myIP, Mask: mask24, Gateway: gwIP, DNS: gwIP})
	s.arp.update(gwIP, peerMAC)
	want := Addr{1, 2, 3, 4}
	go func() {
		f := <-l.sent
		ip := f[14:]
		u := ip[20:]
		if Addr(ip[16:20]) != gwIP || be16(u[2:]) != dnsPort {
			return
		}
		reply := dnsReply(u[8:], 0, want)
		s.Input(ethernet(myMAC, peerMAC, etherIPv4, udpPacket(gwIP, myIP, dnsPort, be16(u[0:]), reply)))
	}()
	got, err := s.LookupHost("example.com", 3*time.Second)
	if err != nil || got != want {
		t.Fatalf("got %v %v", got, err)
	}
	if h.Stats().InUse != 0 {
		t.Errorf("the lookup socket was not freed")
	}
	if a, err := s.LookupHost("8.8.4.4", time.Second); err != nil || a != (Addr{8, 8, 4, 4}) {
		t.Errorf("literal: %v %v", a, err)
	}
}
