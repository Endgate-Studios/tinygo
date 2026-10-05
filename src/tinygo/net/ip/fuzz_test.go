package ip

import (
	"testing"
	"time"

	"runtime/memory/arena"
)

// Fuzz tests of the receive path. They check that no frame makes the stack
// panic or send a frame of a bad size. Run one with, for example:
//
//	go test -run=^$ -fuzz=FuzzTCP -fuzztime=1m

// sinkLink drops the sent frames after a size check. It never blocks, so a
// fuzz input can make the stack send many replies.
type sinkLink struct {
	t    *testing.T
	last []byte
}

func (l *sinkLink) MAC() [6]byte { return myMAC }

func (l *sinkLink) Send(f []byte) error {
	if len(f) < ethLen || len(f) > maxFrame {
		l.t.Fatalf("sent a frame of %d bytes", len(f))
	}
	l.last = append(l.last[:0], f...)
	return nil
}

func newFuzzStack(t *testing.T) (*Stack, *sinkLink, *UDPConn, *TCPListener) {
	h, err := arena.NewHeap(make([]byte, 16<<10))
	if err != nil {
		t.Fatal(err)
	}
	l := &sinkLink{t: t}
	s := New(l, h)
	s.SetConfig(Config{Addr: myIP, Mask: mask24, Gateway: gwIP})
	// No timer goroutine: a fuzz run makes many stacks. FuzzTCP calls
	// tcpTimeouts itself.
	s.tcpTimer = true
	s.arp.update(peerIP, peerMAC)
	s.JoinMulticast(MDNSGroup)
	u, err := s.ListenUDP(7, 256)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := s.ListenTCP(80, 512)
	if err != nil {
		t.Fatal(err)
	}
	return s, l, u, lis
}

// fixL4 sets the checksum of a UDP, TCP or ICMP message.
func fixL4(proto byte, src, dst Addr, p []byte) {
	switch proto {
	case protoUDP:
		if len(p) >= udpLen {
			p[6], p[7] = 0, 0 // no checksum
		}
	case protoTCP:
		if len(p) >= tcpHdrLen {
			p[16], p[17] = 0, 0
			pseudo := append(append(append([]byte(nil), src[:]...), dst[:]...), 0, 6, byte(len(p)>>8), byte(len(p)))
			c := refChecksum(pseudo, p)
			p[16], p[17] = byte(c>>8), byte(c)
		}
	case protoICMP:
		if len(p) >= 4 {
			p[2], p[3] = 0, 0
			c := refChecksum(p)
			p[2], p[3] = byte(c>>8), byte(c)
		}
	}
}

// FuzzInput gives raw frames to the stack.
func FuzzInput(f *testing.F) {
	f.Add(ethernet(broadcastMAC, peerMAC, etherARP, arpPacket(1, peerMAC, peerIP, [6]byte{}, myIP)))
	f.Add(ethernet(myMAC, peerMAC, etherIPv4, udpPacket(peerIP, myIP, 5000, 7, []byte("hello"))))
	f.Add(tcpFrame(peerIP, myIP, seg{4000, 80, 100, 0, tcpSYN, 1000, nil}))
	f.Add(ethernet(myMAC, peerMAC, etherIPv4, ipv4Packet(peerIP, myIP, protoICMP, []byte{8, 0, 0, 0, 0, 1, 0, 1})))
	f.Fuzz(func(t *testing.T, frame []byte) {
		s, _, _, _ := newFuzzStack(t)
		s.Input(frame)
	})
}

// FuzzIPv4 gives the stack a good IPv4 header and good checksums around a
// fuzzed ICMP, UDP or TCP message, so that the input reaches these layers.
func FuzzIPv4(f *testing.F) {
	f.Add(byte(protoUDP), []byte{0x13, 0x88, 0, 7, 0, 13, 0, 0, 'h', 'e', 'l', 'l', 'o'}, false)
	f.Add(byte(protoTCP), tcpFrame(peerIP, myIP, seg{4000, 80, 100, 0, tcpSYN, 1000, nil})[34:], false)
	f.Add(byte(protoICMP), []byte{8, 0, 0, 0, 0, 1, 0, 1, 'x'}, false)
	f.Add(byte(protoUDP), []byte{0x14, 0xe9, 0x14, 0xe9, 0, 20, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0}, true)
	f.Fuzz(func(t *testing.T, proto byte, l4 []byte, multicast bool) {
		if len(l4) > mtu-ipLen {
			return
		}
		proto = []byte{protoICMP, protoTCP, protoUDP}[proto%3]
		s, _, _, _ := newFuzzStack(t)
		dst, mac := myIP, myMAC
		if multicast {
			dst, mac = MDNSGroup, multicastMAC(MDNSGroup)
		}
		p := append([]byte(nil), l4...)
		fixL4(proto, peerIP, dst, p)
		s.Input(ethernet(mac, peerMAC, etherIPv4, ipv4Packet(peerIP, dst, proto, p)))
	})
}

// FuzzTCP opens a connection, then gives it a fuzzed series of segments and
// user calls. Each step takes 4 bytes: the flags, the sequence and the
// acknowledgement offsets, and the data length with the user call.
func FuzzTCP(f *testing.F) {
	f.Add([]byte{tcpACK, 0, 0, 0})
	f.Add([]byte{tcpACK | tcpPSH, 0, 0, 5, tcpACK, 5, 0, 0x40, tcpFIN | tcpACK, 5, 0, 0x80})
	f.Add([]byte{tcpACK, 0, 0, 0xc0, tcpACK, 0, 1, 0, tcpRST, 0, 0, 0})
	f.Fuzz(func(t *testing.T, steps []byte) {
		if len(steps) > 4*64 {
			return
		}
		s, l, _, lis := newFuzzStack(t)
		const sport, irs = 4000, 100
		s.Input(tcpFrame(peerIP, myIP, seg{sport, 80, irs, 0, tcpSYN, 1000, nil}))
		if len(l.last) < 54 {
			t.Fatal("no SYN-ACK")
		}
		iss := be32(l.last[38:])
		seq, ack := uint32(irs+1), iss+1
		var c *TCPConn
		var buf [128]byte
		for ; len(steps) >= 4; steps = steps[4:] {
			flags := steps[0] & 0x1f
			n := int(steps[3] & 0x3f)
			sq := seq + uint32(int8(steps[1]))
			ak := ack + uint32(int8(steps[2]))
			s.Input(tcpFrame(peerIP, myIP, seg{sport, 80, sq, ak, flags, 1000, buf[:n]}))
			if flags&(tcpSYN|tcpRST) == 0 && sq == seq {
				seq += uint32(n)
				if flags&tcpFIN != 0 {
					seq++
				}
			}
			if c == nil {
				c, _ = lis.AcceptDeadline(time.Now())
			}
			if c == nil {
				continue
			}
			now := time.Now()
			switch steps[3] >> 6 {
			case 1:
				c.SetReadDeadline(now)
				c.Read(buf[:])
			case 2:
				c.SetWriteDeadline(now)
				c.Write(buf[:n])
			case 3:
				c.Close()
				c = nil
			}
			s.mu.Lock()
			s.tcpTimeouts()
			s.mu.Unlock()
		}
	})
}

// FuzzDHCPServer gives client messages to the DHCP server.
func FuzzDHCPServer(f *testing.F) {
	mac := [6]byte{2, 0, 0, 0, 0, 0xa}
	f.Add(request(dhcpDiscover, mac, Addr{}, Addr{}))
	f.Add(request(dhcpRequest, mac, Addr{192, 168, 1, 100}, myIP))
	f.Add(request(dhcpRelease, mac, Addr{}, Addr{}))
	f.Fuzz(func(t *testing.T, p []byte) {
		s, _, _, _ := newFuzzStack(t)
		d := &dhcpPool{first: Addr{192, 168, 1, 100}, n: 4, lease: time.Minute}
		// Two messages, so that the second sees the lease of the first.
		for i := 0; i < 2; i++ {
			if n := d.handle(s, p); n > len(d.out) {
				t.Fatalf("reply of %d bytes", n)
			}
		}
	})
}

// FuzzDHCPClient gives server messages to the DHCP client in each state. The
// fixed fields that the client checks first are set, so that the input
// reaches the options.
func FuzzDHCPClient(f *testing.F) {
	ack := make([]byte, dhcpFixedLen)
	ack = append(ack, optMessageType, 1, dhcpAck, optSubnetMask, 4, 255, 255, 255, 0,
		optRouter, 4, 192, 168, 1, 1, optLeaseTime, 4, 0, 0, 0x0e, 0x10, optServerID, 4, 192, 168, 1, 1, optEnd)
	f.Add(byte(0), ack)
	f.Add(byte(1), ack)
	f.Fuzz(func(t *testing.T, state byte, p []byte) {
		s, _, _, _ := newFuzzStack(t)
		d := &s.dhcp
		d.active, d.state, d.xid = true, dhcpState(state%4), 0x1234
		if len(p) >= dhcpFixedLen {
			p[0] = 2
			put32(p[4:], d.xid)
			copy(p[28:34], myMAC[:])
			put32(p[236:], 0x63825363)
		}
		s.mu.Lock()
		d.input(s, p)
		s.mu.Unlock()
	})
}

// FuzzDNS gives messages to the DNS answer parser and the mDNS responder.
func FuzzDNS(f *testing.F) {
	f.Add(mdnsQuery(0, "tinygo-node.local", dnsTypeA))
	f.Add(mdnsQuery(7, "example.com", dnsTypeA))
	ans := make([]byte, 128)
	f.Add(ans[:putMDNSAnswer(ans, "tinygo-node", myIP, 1, nil)])
	f.Fuzz(func(t *testing.T, m []byte) {
		if len(m) >= dnsHdrLen {
			parseAnswer(m)
		}
		if q, ok := mdnsQuestion(m, "tinygo-node"); ok {
			var out [256]byte
			putMDNSAnswer(out[:], "tinygo-node", myIP, 1, q)
		}
	})
}
