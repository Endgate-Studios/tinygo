package ip

import (
	"bytes"
	"io"
	"testing"
	"time"
)

type seg struct {
	sport, dport uint16
	seq, ack     uint32
	flags        byte
	wnd          uint16
	data         []byte
}

func tcpFrame(src, dst Addr, g seg) []byte {
	t := []byte{byte(g.sport >> 8), byte(g.sport), byte(g.dport >> 8), byte(g.dport), 0, 0, 0, 0, 0, 0, 0, 0, 5 << 4, g.flags, byte(g.wnd >> 8), byte(g.wnd), 0, 0, 0, 0}
	put32(t[4:], g.seq)
	put32(t[8:], g.ack)
	t = append(t, g.data...)
	pseudo := append(append(append([]byte(nil), src[:]...), dst[:]...), 0, 6, byte(len(t)>>8), byte(len(t)))
	c := refChecksum(pseudo, t)
	t[16], t[17] = byte(c>>8), byte(c)
	return ethernet(myMAC, peerMAC, etherIPv4, ipv4Packet(src, dst, 6, t))
}

// readSeg takes the next sent frame and checks that it is a TCP segment to
// the peer with good checksums.
func readSeg(t *testing.T, l *fakeLink) seg {
	t.Helper()
	f := l.next(t)
	if [6]byte(f[0:6]) != peerMAC || be16(f[12:]) != etherIPv4 || f[14+9] != 6 {
		t.Fatalf("not a TCP frame to the peer: %x", f)
	}
	ip := f[14:]
	if refChecksum(ip[:20]) != 0 {
		t.Fatal("bad IP checksum")
	}
	tc := ip[20:be16(ip[2:])]
	pseudo := append(append(append([]byte(nil), ip[12:16]...), ip[16:20]...), 0, 6, byte(len(tc)>>8), byte(len(tc)))
	if refChecksum(pseudo, tc) != 0 {
		t.Fatal("bad TCP checksum")
	}
	hl := int(tc[12]>>4) * 4
	return seg{be16(tc[0:]), be16(tc[2:]), be32(tc[4:]), be32(tc[8:]), tc[13], be16(tc[14:]), append([]byte(nil), tc[hl:]...)}
}

func noSeg(t *testing.T, l *fakeLink) {
	t.Helper()
	select {
	case f := <-l.sent:
		t.Fatalf("unexpected frame %x", f)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTCPPassive(t *testing.T) {
	s, l, h := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	lis, err := s.ListenTCP(80, 256)
	if err != nil {
		t.Fatal(err)
	}
	const pISS = 1000
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS, 0, tcpSYN, 4096, nil}))
	synack := readSeg(t, l)
	if synack.flags != tcpSYN|tcpACK || synack.ack != pISS+1 {
		t.Fatalf("SYN-ACK %+v", synack)
	}
	iss := synack.seq
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS + 1, iss + 1, tcpACK, 4096, nil}))
	c, err := lis.Accept()
	if err != nil {
		t.Fatal(err)
	}

	// The peer sends data. The stack ACKs it, and Read gets it.
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS + 1, iss + 1, tcpACK | tcpPSH, 4096, []byte("GET /")}))
	if a := readSeg(t, l); a.flags != tcpACK || a.ack != pISS+6 || a.wnd != 256-5 {
		t.Fatalf("ACK %+v", a)
	}
	buf := make([]byte, 64)
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != "GET /" {
		t.Fatalf("Read %q %v", buf[:n], err)
	}

	// We send data, the peer ACKs it.
	if _, err := c.Write([]byte("200 OK")); err != nil {
		t.Fatal(err)
	}
	d := readSeg(t, l)
	if d.seq != iss+1 || string(d.data) != "200 OK" || d.flags&tcpACK == 0 {
		t.Fatalf("data %+v", d)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS + 6, iss + 7, tcpACK, 4096, nil}))

	// The peer closes first: Read gives EOF.
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS + 6, iss + 7, tcpFIN | tcpACK, 4096, nil}))
	if a := readSeg(t, l); a.ack != pISS+7 {
		t.Fatalf("ACK of FIN %+v", a)
	}
	if n, err := c.Read(buf); n != 0 || err != io.EOF {
		t.Fatalf("Read after FIN: %d %v", n, err)
	}
	c.Close()
	fin := readSeg(t, l)
	if fin.flags&tcpFIN == 0 || fin.seq != iss+7 {
		t.Fatalf("FIN %+v", fin)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS + 7, iss + 8, tcpACK, 4096, nil}))
	if in := h.Stats().InUse; in != 0 {
		t.Errorf("rings not freed: %d bytes in use", in)
	}
	lis.Close()
}

func TestTCPActiveRetransmitAndTimeWait(t *testing.T) {
	s, l, h := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	type result struct {
		c   *TCPConn
		err error
	}
	done := make(chan result, 1)
	go func() {
		c, err := s.DialTCP(peerIP, 8080, 256, 5*time.Second)
		done <- result{c, err}
	}()
	syn := readSeg(t, l)
	if syn.flags != tcpSYN || syn.dport != 8080 {
		t.Fatalf("SYN %+v", syn)
	}
	const pISS = 7000
	s.Input(tcpFrame(peerIP, myIP, seg{8080, syn.sport, pISS, syn.seq + 1, tcpSYN | tcpACK, 4096, nil}))
	if a := readSeg(t, l); a.flags != tcpACK || a.ack != pISS+1 {
		t.Fatalf("ACK %+v", a)
	}
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	c := r.c

	c.Write([]byte("abc"))
	first := readSeg(t, l)
	// No ACK: the same bytes come again after the timeout.
	again := readSeg(t, l)
	if again.seq != first.seq || !bytes.Equal(again.data, first.data) {
		t.Fatalf("retransmission %+v, first %+v", again, first)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{8080, syn.sport, pISS + 1, first.seq + 3, tcpACK, 4096, nil}))

	// We close first: FIN-WAIT-1, FIN-WAIT-2, TIME-WAIT, closed.
	c.Close()
	fin := readSeg(t, l)
	if fin.flags&tcpFIN == 0 {
		t.Fatalf("FIN %+v", fin)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{8080, syn.sport, pISS + 1, fin.seq + 1, tcpACK, 4096, nil}))
	s.Input(tcpFrame(peerIP, myIP, seg{8080, syn.sport, pISS + 1, fin.seq + 1, tcpFIN | tcpACK, 4096, nil}))
	if a := readSeg(t, l); a.ack != pISS+2 {
		t.Fatalf("ACK of the peer FIN %+v", a)
	}
	// TIME-WAIT keeps the slot but not the rings.
	if in := h.Stats().InUse; in != 0 {
		t.Errorf("rings not freed in TIME-WAIT: %d bytes", in)
	}
	if n := tcpSlotsInUse(s); n != 1 {
		t.Fatalf("%d slots in use in TIME-WAIT, want 1", n)
	}
	time.Sleep(timeWaitTime + 300*time.Millisecond)
	if n := tcpSlotsInUse(s); n != 0 {
		t.Errorf("%d slots in use after TIME-WAIT", n)
	}
}

func TestTCPPeerWindow(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	lis, _ := s.ListenTCP(81, 4096)
	s.Input(tcpFrame(peerIP, myIP, seg{6000, 81, 1, 0, tcpSYN, 1000, nil}))
	iss := readSeg(t, l).seq
	s.Input(tcpFrame(peerIP, myIP, seg{6000, 81, 2, iss + 1, tcpACK, 1000, nil}))
	c, _ := lis.Accept()
	msg := make([]byte, 2000)
	for i := range msg {
		msg[i] = byte(i)
	}
	go c.Write(msg)
	// The peer window is 1000 bytes and our MSS toward it is 536.
	a := readSeg(t, l)
	b := readSeg(t, l)
	if len(a.data) != 536 || len(b.data) != 1000-536 {
		t.Fatalf("segments of %d and %d bytes", len(a.data), len(b.data))
	}
	noSeg(t, l)
	// The ACK opens the window again.
	s.Input(tcpFrame(peerIP, myIP, seg{6000, 81, 2, iss + 1001, tcpACK, 1000, nil}))
	got := append(append([]byte(nil), a.data...), b.data...)
	for len(got) < len(msg) {
		x := readSeg(t, l)
		got = append(got, x.data...)
		if len(got) == 2000 || len(got)%1000 == 0 {
			s.Input(tcpFrame(peerIP, myIP, seg{6000, 81, 2, iss + 1 + uint32(len(got)), tcpACK, 1000, nil}))
		}
	}
	if !bytes.Equal(got, msg) {
		t.Fatal("the data came out different")
	}
}

func TestTCPResetClosedPort(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.Input(tcpFrame(peerIP, myIP, seg{4000, 99, 500, 0, tcpSYN, 1000, nil}))
	r := readSeg(t, l)
	if r.flags != tcpRST|tcpACK || r.ack != 501 || r.seq != 0 {
		t.Fatalf("RST %+v", r)
	}
}

// TestTCPListenerFull drops a SYN when no connection has memory, so that the
// peer sends it again, and does not refuse it.
func TestTCPListenerFull(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	s.ListenTCP(83, 6<<10) // the rings of one connection fill the heap
	s.Input(tcpFrame(peerIP, myIP, seg{7100, 83, 100, 0, tcpSYN, 1000, nil}))
	if r := readSeg(t, l); r.flags != tcpSYN|tcpACK {
		t.Fatalf("first SYN: %+v, want a SYN-ACK", r)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{7101, 83, 200, 0, tcpSYN, 1000, nil}))
	noSeg(t, l)
}

func TestTCPTimeWaitSlotReuse(t *testing.T) {
	s, l, h := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	lis, _ := s.ListenTCP(82, 128)
	// Each connection: the peer opens, we close first, the peer closes. The
	// slot stays in TIME-WAIT. More connections than slots must still work.
	for i := 0; i < 2*maxTCPConns; i++ {
		sport := uint16(7000 + i)
		s.Input(tcpFrame(peerIP, myIP, seg{sport, 82, 100, 0, tcpSYN, 1000, nil}))
		synack := readSeg(t, l)
		if synack.flags != tcpSYN|tcpACK {
			t.Fatalf("connection %d: %+v, want a SYN-ACK", i, synack)
		}
		s.Input(tcpFrame(peerIP, myIP, seg{sport, 82, 101, synack.seq + 1, tcpACK, 1000, nil}))
		c, err := lis.Accept()
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
		fin := readSeg(t, l)
		s.Input(tcpFrame(peerIP, myIP, seg{sport, 82, 101, fin.seq + 1, tcpFIN | tcpACK, 1000, nil}))
		readSeg(t, l) // our ACK of the peer FIN
	}
	// At most maxTCPConns connections hold rings: two of 128 bytes each, and
	// a block tag of 16 bytes for each ring.
	if in := h.Stats().InUse; in > maxTCPConns*2*(128+16) {
		t.Errorf("%d bytes in use, more than %d connections", in, maxTCPConns)
	}
}

func tcpSlotsInUse(s *Stack) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.tcp {
		if c != nil {
			n++
		}
	}
	return n
}

// accepted opens a connection from the peer, with pISS as its ISS, and gives
// it and our ISS.
func accepted(t *testing.T, s *Stack, l *fakeLink, ring int, pISS uint32) (*TCPConn, uint32) {
	t.Helper()
	s.arp.update(peerIP, peerMAC)
	lis, err := s.ListenTCP(80, ring)
	if err != nil {
		t.Fatal(err)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS, 0, tcpSYN, 4096, nil}))
	iss := readSeg(t, l).seq
	s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, pISS + 1, iss + 1, tcpACK, 4096, nil}))
	c, err := lis.Accept()
	if err != nil {
		t.Fatal(err)
	}
	return c, iss
}

func TestTCPOutOfOrderKept(t *testing.T) {
	s, l, _ := newTestStack(t)
	const p = 1000
	c, iss := accepted(t, s, l, 256, p)
	send := func(off int, data string) seg {
		t.Helper()
		s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, p + 1 + uint32(off), iss + 1, tcpACK, 4096, []byte(data)}))
		return readSeg(t, l)
	}
	stream := "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeee"
	part := func(i int) string { return stream[i*10 : i*10+10] }

	if a := send(0, part(0)); a.ack != p+11 {
		t.Fatalf("ACK of a: %+v", a)
	}
	// c, e and d come before b: each is ACKed as a duplicate.
	for _, i := range []int{2, 4, 3} {
		if a := send(i*10, part(i)); a.ack != p+11 || a.wnd != 256-10 {
			t.Fatalf("ACK of %c: %+v", stream[i*10], a)
		}
	}
	if c.nHeld != 1 || c.held[0] != (span{p + 21, p + 51}) {
		t.Fatalf("held %v", c.held[:c.nHeld])
	}
	// b comes again from inside a: all of it is in now.
	if a := send(5, stream[5:20]); a.ack != p+51 || a.wnd != 256-50 {
		t.Fatalf("ACK after the gap filled: %+v", a)
	}
	buf := make([]byte, 64)
	if n, err := c.Read(buf); err != nil || string(buf[:n]) != stream {
		t.Fatalf("Read %q %v", buf[:n], err)
	}
	if s.stats.TCPOutOfOrder != 3 || s.stats.TCPOutOfOrderDropped != 0 || s.stats.TCPBytesIn != 50 {
		t.Errorf("stats %+v", s.stats)
	}

	// Past the window, or a fifth gap, is dropped.
	at := 50
	send(at+300, "x")
	for i := 1; i <= maxHeld+1; i++ {
		send(at+i*10, "y")
	}
	if s.stats.TCPOutOfOrderDropped != 2 || c.nHeld != maxHeld {
		t.Errorf("dropped %d, held %d", s.stats.TCPOutOfOrderDropped, c.nHeld)
	}
}

// TestTCPOutOfOrderStream sends a stream through a small ring in segments
// that come in any order within the window, some twice and some overlapping,
// reading as it goes: what Read gives must be the stream.
func TestTCPOutOfOrderStream(t *testing.T) {
	s, l, _ := newTestStack(t)
	const p = 0xfffff000 // the sequence numbers wrap
	c, iss := accepted(t, s, l, 200, p)
	stream := make([]byte, 20000)
	rng := uint32(1)
	rand := func(n int) int {
		rng = rng*1103515245 + 12345
		return int(rng>>8) % n
	}
	for i := range stream {
		stream[i] = byte(rand(256))
	}
	var got []byte
	acked, wnd := 0, 200
	buf := make([]byte, 100)
	for len(got) < len(stream) {
		if acked < len(stream) && wnd > 0 {
			off := acked + rand(wnd)
			n := min(1+rand(40), len(stream)-off, acked+wnd-off)
			if n > 0 {
				s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, p + 1 + uint32(off), iss + 1, tcpACK, 4096, stream[off : off+n]}))
				a := readSeg(t, l)
				acked, wnd = int(a.ack-(p+1)), int(a.wnd)
			}
		}
		s.mu.Lock()
		ready := c.rcvLen > 0
		s.mu.Unlock()
		if wnd == 0 && !ready && acked < len(stream) {
			// The window opened, but less than a segment: the stack doesn't
			// say so, and a peer probes, a byte at a time.
			s.Input(tcpFrame(peerIP, myIP, seg{5555, 80, p + 1 + uint32(acked), iss + 1, tcpACK, 4096, stream[acked : acked+1]}))
			a := readSeg(t, l)
			acked, wnd = int(a.ack-(p+1)), int(a.wnd)
		}
		if rand(3) == 0 || wnd == 0 {
			if ready {
				n, err := c.Read(buf[:1+rand(len(buf))])
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, buf[:n]...)
				for len(l.sent) > 0 { // a window update
					a := readSeg(t, l)
					acked, wnd = int(a.ack-(p+1)), int(a.wnd)
				}
			}
		}
	}
	if !bytes.Equal(got, stream) {
		t.Fatalf("Read differs from the stream from byte %d", func() int {
			for i := range got {
				if got[i] != stream[i] {
					return i
				}
			}
			return len(got)
		}())
	}
	if s.stats.TCPOutOfOrder == 0 {
		t.Error("no segment came out of order")
	}
	t.Logf("%+v", s.stats)
}

// A segment the link refuses (its transmit buffers full) goes again at the
// next timer tick, not the retransmission timeout, and the data arrives
// whole and in order.
func TestTCPLinkFull(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	lis, _ := s.ListenTCP(82, 4096)
	s.Input(tcpFrame(peerIP, myIP, seg{7000, 82, 1, 0, tcpSYN, 4096, nil}))
	iss := readSeg(t, l).seq
	s.Input(tcpFrame(peerIP, myIP, seg{7000, 82, 2, iss + 1, tcpACK, 4096, nil}))
	c, _ := lis.Accept()
	msg := make([]byte, 3*536)
	for i := range msg {
		msg[i] = byte(i * 7)
	}
	l.refuse.Store(2) // the first segment, and its first try again
	start := time.Now()
	go c.Write(msg)
	var got []byte
	for len(got) < len(msg) {
		x := readSeg(t, l)
		if x.seq != iss+1+uint32(len(got)) {
			t.Fatalf("segment at %d, want %d", x.seq-iss-1, len(got))
		}
		got = append(got, x.data...)
	}
	if took := time.Since(start); took >= rtoInitial {
		t.Errorf("took %v: the retransmission timeout, not the tick", took)
	}
	if !bytes.Equal(got, msg) {
		t.Fatal("the data came out different")
	}
	if n := s.Stats().TCPTxFull; n != 2 {
		t.Errorf("TCPTxFull %d, want 2", n)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{7000, 82, 2, iss + 1 + uint32(len(msg)), tcpACK, 4096, nil}))
}

// Three duplicate ACKs send again from the first unacknowledged byte at
// once, without waiting for the retransmission timeout.
func TestTCPFastRetransmit(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	lis, _ := s.ListenTCP(83, 4096)
	s.Input(tcpFrame(peerIP, myIP, seg{7100, 83, 1, 0, tcpSYN, 4096, nil}))
	iss := readSeg(t, l).seq
	s.Input(tcpFrame(peerIP, myIP, seg{7100, 83, 2, iss + 1, tcpACK, 4096, nil}))
	c, _ := lis.Accept()
	msg := make([]byte, 4*536)
	for i := range msg {
		msg[i] = byte(i * 3)
	}
	start := time.Now()
	go c.Write(msg)
	for range 4 {
		readSeg(t, l)
	}
	// The first segment is lost: the peer ACKs only up to it, for each of
	// the other three.
	for range 3 {
		s.Input(tcpFrame(peerIP, myIP, seg{7100, 83, 2, iss + 1, tcpACK, 4096, nil}))
	}
	again := readSeg(t, l)
	if again.seq != iss+1 || !bytes.Equal(again.data, msg[:536]) {
		t.Fatalf("sent again at %d, %d bytes", again.seq-iss-1, len(again.data))
	}
	if took := time.Since(start); took >= rtoInitial {
		t.Errorf("took %v: the retransmission timeout, not fast retransmit", took)
	}
	if n := s.Stats().TCPFastRetransmits; n != 1 {
		t.Errorf("TCPFastRetransmits %d, want 1", n)
	}
	s.Input(tcpFrame(peerIP, myIP, seg{7100, 83, 2, iss + 1 + uint32(len(msg)), tcpACK, 4096, nil}))
}
