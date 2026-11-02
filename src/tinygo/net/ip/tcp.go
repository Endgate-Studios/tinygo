package ip

import (
	"io"
	"time"

	"runtime/memory"
)

// TCP of RFC 9293. Segments past a gap are kept in the receive ring, where
// they will go, for up to maxHeld ranges, and join the data once the gap is
// filled; the peer resends the one lost segment, not all after it. The window
// is not scaled, and there is no SACK. Retransmission is go-back-N with a
// doubling timeout.

const (
	protoTCP     = 6
	tcpHdrLen    = 20
	tcpMSS       = mtu - ipLen - tcpHdrLen
	maxTCPConns  = 8
	maxHeld      = 4 // ranges of data past a gap that a connection keeps
	tcpTick      = 50 * time.Millisecond
	rtoInitial   = time.Second
	rtoMax       = 16 * time.Second
	maxRetries   = 6
	timeWaitTime = 2 * time.Second

	// DefaultTCPBuffer is the size of the send and the receive ring of a
	// connection.
	DefaultTCPBuffer = 2048

	tcpFIN = 1
	tcpSYN = 2
	tcpRST = 4
	tcpPSH = 8
	tcpACK = 16
)

type tcpState uint8

const (
	tcpClosed tcpState = iota
	tcpSynSent
	tcpSynRcvd
	tcpEstablished
	tcpFinWait1
	tcpFinWait2
	tcpCloseWait
	tcpClosing
	tcpLastAck
	tcpTimeWait
)

const (
	ErrRefused Error = "ip: connection refused"
	ErrReset   Error = "ip: connection reset"
	ErrTimeout Error = "ip: connection timed out"
)

// TCPConn is a TCP connection. Do not use it after Close: the stack reuses it
// for a new connection.
type TCPConn struct {
	s     *Stack
	state tcpState
	lport uint16
	rport uint16
	raddr Addr
	mac   [6]byte // of the next hop, so Input does not wait for ARP
	lis   *TCPListener
	rd    chan struct{} // data, EOF or an error for Read
	wr    chan struct{} // ring space or an error for Write

	iss, sndUna, sndNxt uint32
	sndWnd              int
	mss                 int
	rcvNxt              uint32

	snd       []byte // unacknowledged and unsent data
	sndHead   int
	sndLen    int
	sent      int // bytes of snd that are sent and not acknowledged
	rcv       []byte
	rcvHead   int
	rcvLen    int
	held      [maxHeld]span // data past a gap, in rcv already; sorted, apart
	nHeld     int
	wndClosed bool // the last ACK told a window of 0

	finQueued, finSent bool
	peerFin            bool
	userClosed         bool
	err                error

	// txFull: the link refused the segment at sent (its transmit buffers
	// were full), and the rest waits for the next ACK or timer tick.
	txFull bool
	// dupAcks counts the duplicate ACKs since the last that acknowledged
	// anything new.
	dupAcks int

	rtoAt    time.Time
	rto      time.Duration
	retries  int
	waitDone time.Time

	readDeadline, writeDeadline time.Time
}

// SetReadDeadline sets when Read gives up with ErrDeadline. Zero waits
// forever.
func (c *TCPConn) SetReadDeadline(t time.Time) {
	c.s.mu.Lock()
	c.readDeadline = t
	c.s.mu.Unlock()
}

// SetWriteDeadline sets when Write gives up with ErrDeadline. Zero waits
// forever.
func (c *TCPConn) SetWriteDeadline(t time.Time) {
	c.s.mu.Lock()
	c.writeDeadline = t
	c.s.mu.Unlock()
}

// span is the sequence numbers [start, end).
type span struct{ start, end uint32 }

// TCPListener accepts connections on a local port.
type TCPListener struct {
	s       *Stack
	port    uint16
	bufSize int
	ev      chan struct{}
	shut    bool
}

func (s *Stack) startTCPTimer() {
	if s.tcpTimer {
		return
	}
	s.tcpTimer = true
	go func() {
		for {
			time.Sleep(tcpTick) // allocates nothing, unlike a Ticker
			s.mu.Lock()
			s.tcpTimeouts()
			s.mu.Unlock()
		}
	}()
}

// ListenTCP accepts connections on port. Each connection has a send and a
// receive ring of bufSize bytes, or DefaultTCPBuffer when bufSize is 0.
func (s *Stack) ListenTCP(port uint16, bufSize int) (*TCPListener, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.tcpListeners {
		if l != nil && l.port == port {
			return nil, ErrPortInUse
		}
	}
	for i, l := range s.tcpListeners {
		if l == nil {
			if bufSize == 0 {
				bufSize = DefaultTCPBuffer
			}
			nl := &TCPListener{s: s, port: port, bufSize: bufSize, ev: make(chan struct{}, 1)}
			s.tcpListeners[i] = nl
			s.startTCPTimer()
			return nl, nil
		}
	}
	return nil, ErrNoSockets
}

// Accept waits for a connection.
func (l *TCPListener) Accept() (*TCPConn, error) {
	return l.AcceptDeadline(time.Time{})
}

// AcceptDeadline is Accept that gives ErrDeadline after deadline. A zero
// deadline waits forever.
func (l *TCPListener) AcceptDeadline(deadline time.Time) (*TCPConn, error) {
	s := l.s
	for {
		s.mu.Lock()
		if l.shut {
			s.mu.Unlock()
			return nil, ErrClosed
		}
		for _, c := range s.tcp {
			if c != nil && c.lis == l && c.state != tcpSynRcvd {
				c.lis = nil
				s.mu.Unlock()
				return c, nil
			}
		}
		s.mu.Unlock()
		if !wait(l.ev, deadline) {
			return nil, ErrDeadline
		}
	}
}

// Close stops accepting. Connections that are not accepted are reset.
func (l *TCPListener) Close() error {
	s := l.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if l.shut {
		return ErrClosed
	}
	l.shut = true
	for i, x := range s.tcpListeners {
		if x == l {
			s.tcpListeners[i] = nil
		}
	}
	for _, c := range s.tcp {
		if c != nil && c.lis == l {
			c.lis = nil
			c.userClosed = true
			s.tcpReset(c, ErrReset)
		}
	}
	signal(l.ev)
	return nil
}

// DialTCP connects to the address and port. bufSize is as for ListenTCP.
func (s *Stack) DialTCP(addr Addr, port uint16, bufSize int, timeout time.Duration) (*TCPConn, error) {
	s.mu.Lock()
	if s.cfg.Addr.IsZero() {
		s.mu.Unlock()
		return nil, ErrNoAddress
	}
	mac, err := s.resolve(addr)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	lport := s.freeTCPPort()
	c, err := s.newTCPConn(lport, addr, port, mac, bufSize)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	c.state = tcpSynSent
	c.sndNxt = c.iss + 1
	s.tcpSendSYN(c)
	s.startTCPTimer()
	s.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		state, err := c.state, c.err
		s.mu.Unlock()
		switch {
		case state == tcpEstablished:
			return c, nil
		case err != nil:
			c.Close()
			return nil, err
		case time.Now().After(deadline):
			c.Close()
			return nil, ErrTimeout
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// freeTCPPort gives a free local port. The first one is random (RFC 6056),
// so a new boot does not reuse the ports of the last one.
func (s *Stack) freeTCPPort() uint16 {
	if s.tcpPort == 0 {
		s.isnSeed = s.isnSeed*1103515245 + uint32(time.Now().UnixNano())
		s.tcpPort = ephemeralBase + uint16(s.isnSeed>>16)%(0xffff-ephemeralBase)
	}
	for {
		s.tcpPort++
		if s.tcpPort < ephemeralBase {
			s.tcpPort = ephemeralBase
		}
		used := false
		for _, c := range s.tcp {
			if c != nil && c.lport == s.tcpPort {
				used = true
			}
		}
		if !used {
			return s.tcpPort
		}
	}
}

// newTCPConn takes a slot and the rings. The caller holds s.mu.
func (s *Stack) newTCPConn(lport uint16, raddr Addr, rport uint16, mac [6]byte, bufSize int) (*TCPConn, error) {
	slot := s.freeTCPSlot()
	if slot < 0 {
		return nil, ErrNoSockets
	}
	if bufSize == 0 {
		bufSize = DefaultTCPBuffer
	}
	snd := memory.MakeSlice[byte](s.mem, bufSize)
	if snd == nil {
		return nil, ErrNoMemory
	}
	rcv := memory.MakeSlice[byte](s.mem, bufSize)
	if rcv == nil {
		memory.FreeSlice(s.mem, snd)
		return nil, ErrNoMemory
	}
	s.isnSeed = s.isnSeed*1103515245 + uint32(time.Now().UnixNano())
	// The connection objects and their channels are reused, so a connection
	// does not allocate on the GC heap.
	c := &s.tcpStore[slot]
	rd, wr := c.rd, c.wr
	if rd == nil {
		rd, wr = make(chan struct{}, 1), make(chan struct{}, 1)
	}
	drain(rd)
	drain(wr)
	*c = TCPConn{
		s: s, lport: lport, raddr: raddr, rport: rport, mac: mac,
		rd: rd, wr: wr, snd: snd, rcv: rcv,
		iss: s.isnSeed, mss: 536, rto: rtoInitial,
	}
	c.sndUna, c.sndNxt = c.iss, c.iss
	s.tcp[slot] = c
	return c, nil
}

// freeTCPSlot gives a free slot. When all are in use, it ends the oldest
// connection in TIME-WAIT. The user closed it and the stack sent its last
// ACK, so only a late duplicate segment can still come. The caller holds s.mu.
func (s *Stack) freeTCPSlot() int {
	oldest := -1
	for i, c := range s.tcp {
		if c == nil {
			return i
		}
		if c.state == tcpTimeWait && (oldest < 0 || c.waitDone.Before(s.tcp[oldest].waitDone)) {
			oldest = i
		}
	}
	if oldest >= 0 {
		s.tcpDone(s.tcp[oldest], nil)
		if s.tcp[oldest] == nil {
			return oldest
		}
	}
	return -1
}

// release frees the rings and the slot once the protocol and the user are
// both done. The caller holds s.mu.
func (s *Stack) release(c *TCPConn) {
	if !c.userClosed || (c.state != tcpClosed) {
		return
	}
	for i, x := range s.tcp {
		if x == c {
			s.tcp[i] = nil
		}
	}
	s.freeRings(c)
}

// RemoteAddr gives the address and the port of the peer.
func (c *TCPConn) RemoteAddr() (Addr, uint16) { return c.raddr, c.rport }

// Read waits for data. It gives io.EOF after the peer closed its side.
func (c *TCPConn) Read(b []byte) (int, error) {
	s := c.s
	for {
		s.mu.Lock()
		if c.rcvLen > 0 && len(b) > 0 {
			n := min(len(b), c.rcvLen)
			i := c.rcvHead
			m := copy(b[:n], c.rcv[i:])
			copy(b[m:n], c.rcv)
			c.rcvHead = (c.rcvHead + n) % len(c.rcv)
			c.rcvLen -= n
			// Tell the peer that the window opened again.
			if c.wndClosed && c.window() >= c.mss {
				s.tcpSend(c, tcpACK, c.sndNxt, 0, 0)
			}
			s.mu.Unlock()
			return n, nil
		}
		err := c.err
		if err == nil && (c.peerFin || c.userClosed) {
			err = io.EOF
		}
		deadline := c.readDeadline
		s.mu.Unlock()
		if err != nil {
			return 0, err
		}
		if !wait(c.rd, deadline) {
			return 0, ErrDeadline
		}
	}
}

// Write copies b into the send ring and sends it. It waits when the ring is
// full.
func (c *TCPConn) Write(b []byte) (int, error) {
	s := c.s
	done := 0
	for done < len(b) {
		s.mu.Lock()
		if c.err != nil || c.finQueued || c.userClosed {
			err := c.err
			if err == nil {
				err = ErrClosed
			}
			s.mu.Unlock()
			return done, err
		}
		free := len(c.snd) - c.sndLen
		if free > 0 {
			n := min(free, len(b)-done)
			i := (c.sndHead + c.sndLen) % len(c.snd)
			m := copy(c.snd[i:], b[done:done+n])
			copy(c.snd, b[done+m:done+n])
			c.sndLen += n
			done += n
			s.tcpOutput(c)
		}
		deadline := c.writeDeadline
		s.mu.Unlock()
		if done < len(b) && !wait(c.wr, deadline) {
			return done, ErrDeadline
		}
	}
	return done, nil
}

// Close sends the rest of the data and a FIN. The rings are freed when the
// peer has closed too.
func (c *TCPConn) Close() error {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.userClosed {
		return ErrClosed
	}
	c.userClosed = true
	switch c.state {
	case tcpSynSent, tcpSynRcvd:
		s.tcpReset(c, ErrClosed)
	case tcpEstablished:
		c.finQueued = true
		c.state = tcpFinWait1
		s.tcpOutput(c)
	case tcpCloseWait:
		c.finQueued = true
		c.state = tcpLastAck
		s.tcpOutput(c)
	}
	s.release(c)
	c.wake()
	return nil
}

// window gives the free space of the receive ring for the TCP header.
func (c *TCPConn) window() int {
	return min(len(c.rcv)-c.rcvLen, 0xffff)
}

// wake signals both a blocked Read and a blocked Write.
func (c *TCPConn) wake() {
	signal(c.rd)
	signal(c.wr)
}

func drain(ch chan struct{}) {
	select {
	case <-ch:
	default:
	}
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// tcpOutput sends the unsent data that the peer window allows, then the FIN
// when the data is done. The caller holds s.mu.
func (s *Stack) tcpOutput(c *TCPConn) {
	if c.state != tcpEstablished && c.state != tcpCloseWait && c.state != tcpFinWait1 && c.state != tcpLastAck && c.state != tcpClosing {
		return
	}
	c.txFull = false
	for c.sent < c.sndLen {
		n := min(c.sndLen-c.sent, c.mss, c.sndWnd-c.sent)
		if n <= 0 {
			break
		}
		// A segment the link refuses isn't sent: counted as sent, it
		// would be lost, and only the retransmission timeout (a second
		// at least) would send it again, and all after it. The radio
		// refuses at once when its transmit buffers are full, which a
		// ring's worth of segments at a time does. So it stops here, and
		// the next ACK, or the next tick, sends it.
		if s.tcpSend(c, tcpACK|tcpPSH, c.sndNxt, c.sent, n) != nil {
			c.txFull = true
			s.stats.TCPTxFull++
			break
		}
		c.sent += n
		c.sndNxt += uint32(n)
		s.armRTO(c)
	}
	if c.sent < c.sndLen {
		s.armRTO(c) // the peer window is full: the timeout probes it
	}
	if c.finQueued && !c.finSent && c.sent == c.sndLen {
		if s.tcpSend(c, tcpFIN|tcpACK, c.sndNxt, 0, 0) != nil {
			c.txFull = true
			return
		}
		c.finSent = true
		c.sndNxt++
		s.armRTO(c)
	}
}

func (s *Stack) armRTO(c *TCPConn) {
	if c.rtoAt.IsZero() {
		c.rtoAt = time.Now().Add(c.rto)
	}
}

func (s *Stack) tcpSendSYN(c *TCPConn) {
	flags := byte(tcpSYN)
	if c.state == tcpSynRcvd {
		flags |= tcpACK
	}
	s.tcpSend(c, flags, c.iss, 0, 0)
	s.armRTO(c)
}

// tcpSend writes a segment into s.tx and sends it to c.mac. The data is n
// bytes of the send ring at offset off after its head. The caller holds s.mu.
// It gives the link's error: a segment it refused wasn't sent.
func (s *Stack) tcpSend(c *TCPConn, flags byte, seq uint32, off, n int) error {
	hl := tcpHdrLen
	if flags&tcpSYN != 0 {
		hl += 4 // MSS option
	}
	t := s.tx[ethLen+ipLen : ethLen+ipLen+hl+n]
	put16(t[0:], c.lport)
	put16(t[2:], c.rport)
	put32(t[4:], seq)
	ack := uint32(0)
	if flags&tcpACK != 0 {
		ack = c.rcvNxt
	}
	put32(t[8:], ack)
	t[12], t[13] = byte(hl/4)<<4, flags
	wnd := c.window()
	c.wndClosed = wnd < c.mss
	put16(t[14:], uint16(wnd))
	t[16], t[17], t[18], t[19] = 0, 0, 0, 0
	if flags&tcpSYN != 0 {
		t[20], t[21] = 2, 4
		put16(t[22:], tcpMSS)
	}
	if n > 0 {
		i := (c.sndHead + off) % len(c.snd)
		m := copy(t[hl:], c.snd[i:min(i+n, len(c.snd))])
		copy(t[hl+m:], c.snd[:n-m])
	}
	put16(t[16:], fold(checksum(pseudoHeader(s.cfg.Addr, c.raddr, protoTCP, len(t)), t)))
	s.putIPv4Header(s.cfg.Addr, c.raddr, protoTCP, len(t))
	return s.sendEthernet(c.mac, etherIPv4, ipLen+len(t))
}

// tcpReset sends a RST and ends the connection with err. The caller holds
// s.mu.
func (s *Stack) tcpReset(c *TCPConn, err error) {
	if c.state != tcpClosed && c.state != tcpTimeWait {
		s.tcpSend(c, tcpRST|tcpACK, c.sndNxt, 0, 0)
	}
	s.tcpDone(c, err)
}

// tcpDone ends the connection. The caller holds s.mu.
func (s *Stack) tcpDone(c *TCPConn, err error) {
	if c.err == nil {
		c.err = err
	}
	c.state = tcpClosed
	c.rtoAt = time.Time{}
	c.wake()
	s.release(c)
}

// tcpTimeouts retransmits and ends the TIME-WAIT state. The caller holds
// s.mu.
func (s *Stack) tcpTimeouts() {
	now := time.Now()
	for _, c := range s.tcp {
		if c == nil {
			continue
		}
		if c.state == tcpTimeWait && now.After(c.waitDone) {
			s.tcpDone(c, nil)
			continue
		}
		if c.txFull {
			s.tcpOutput(c) // the link refused a segment: again
		}
		if c.rtoAt.IsZero() || now.Before(c.rtoAt) {
			continue
		}
		c.retries++
		if c.retries > maxRetries {
			s.tcpReset(c, ErrTimeout)
			continue
		}
		c.rto = min(2*c.rto, rtoMax)
		c.rtoAt = time.Time{}
		switch c.state {
		case tcpSynSent, tcpSynRcvd:
			s.tcpSendSYN(c)
		default:
			// Go back to the first unacknowledged byte.
			c.sent = 0
			c.sndNxt = c.sndUna
			c.finSent = false
			if c.sndWnd == 0 {
				c.sndWnd = 1 // probe a closed window
			}
			s.tcpOutput(c)
		}
	}
}

// seqLT reports whether a is before b in sequence space.
func seqLT(a, b uint32) bool { return int32(a-b) < 0 }

func (s *Stack) inputTCP(srcMAC [6]byte, src, dst Addr, p []byte) {
	if len(p) < tcpHdrLen || dst != s.cfg.Addr {
		return
	}
	hl := int(p[12]>>4) * 4
	if hl < tcpHdrLen || hl > len(p) || fold(checksum(pseudoHeader(src, dst, protoTCP, len(p)), p)) != 0 {
		s.stats.Dropped++
		return
	}
	sport, dport := be16(p[0:]), be16(p[2:])
	seq, ack := be32(p[4:]), be32(p[8:])
	flags := p[13]
	wnd := int(be16(p[14:]))
	data := p[hl:]
	mss := 536
	for o := p[tcpHdrLen:hl]; len(o) > 0; {
		switch {
		case o[0] == 0:
			o = nil
		case o[0] == 1:
			o = o[1:]
		case len(o) < 2 || int(o[1]) < 2 || int(o[1]) > len(o):
			o = nil
		default:
			if o[0] == 2 && o[1] == 4 {
				mss = int(be16(o[2:]))
			}
			o = o[o[1]:]
		}
	}

	var c *TCPConn
	for _, x := range s.tcp {
		if x != nil && x.lport == dport && x.rport == sport && x.raddr == src {
			c = x
			break
		}
	}
	if c == nil {
		s.tcpNoConn(srcMAC, src, sport, dport, seq, ack, flags, len(data), mss)
		return
	}
	s.stats.TCPSegments++

	if flags&tcpRST != 0 {
		if c.state == tcpSynSent {
			if flags&tcpACK != 0 && ack == c.iss+1 {
				s.tcpDone(c, ErrRefused)
			}
			return
		}
		if seq == c.rcvNxt {
			s.tcpDone(c, ErrReset)
		}
		return
	}

	switch c.state {
	case tcpSynSent:
		if flags&(tcpSYN|tcpACK) != tcpSYN|tcpACK || ack != c.iss+1 {
			return
		}
		c.rcvNxt = seq + 1
		c.sndUna = ack
		c.sndWnd, c.mss = wnd, min(mss, tcpMSS)
		c.state = tcpEstablished
		c.rtoAt, c.retries, c.rto = time.Time{}, 0, rtoInitial
		s.tcpSend(c, tcpACK, c.sndNxt, 0, 0)
		c.wake()
		return
	case tcpSynRcvd:
		if flags&tcpSYN != 0 && seq+1 == c.rcvNxt {
			s.tcpSendSYN(c) // our SYN-ACK was lost
			return
		}
		if flags&tcpACK == 0 || ack != c.iss+1 {
			return
		}
		c.sndUna = ack
		c.sndWnd = wnd
		c.state = tcpEstablished
		c.rtoAt, c.retries, c.rto = time.Time{}, 0, rtoInitial
		if c.lis != nil {
			signal(c.lis.ev)
		}
	}
	if flags&tcpACK == 0 {
		return
	}

	// The acknowledgment.
	if seqLT(c.sndUna, ack) && !seqLT(c.sndNxt, ack) {
		acked := int(ack - c.sndUna)
		finAcked := c.finSent && ack == c.sndNxt
		if finAcked {
			acked--
		}
		c.sndHead = (c.sndHead + acked) % len(c.snd)
		c.sndLen -= acked
		c.sent -= acked
		c.sndUna = ack
		c.retries, c.rto, c.rtoAt = 0, rtoInitial, time.Time{}
		c.dupAcks = 0
		if c.sndUna != c.sndNxt {
			s.armRTO(c)
		}
		if finAcked {
			switch c.state {
			case tcpFinWait1:
				c.state = tcpFinWait2
			case tcpClosing:
				s.enterTimeWait(c)
			case tcpLastAck:
				s.tcpDone(c, nil)
				return
			}
		}
		c.wake()
	} else if ack == c.sndUna && c.sndUna != c.sndNxt && len(data) == 0 && flags&(tcpSYN|tcpFIN) == 0 && wnd == c.sndWnd {
		// A duplicate ACK: the peer has had a segment past one it hasn't,
		// lost on the way. At the third, RFC 5681's fast retransmit, the
		// stack goes back to the first unacknowledged byte and sends
		// again, as the timeout would, but now: the timeout is a second
		// at least, and doubles each time it fires, so a few losses in a
		// long transfer would stall it for seconds.
		c.dupAcks++
		if c.dupAcks == 3 {
			s.stats.TCPFastRetransmits++
			c.sent = 0
			c.sndNxt = c.sndUna
			c.finSent = false
		}
	}
	if ack == c.sndUna {
		c.sndWnd = wnd
	}

	// The data and the FIN.
	needACK := len(data) > 0 || flags&tcpFIN != 0
	if seqLT(seq, c.rcvNxt) && seqLT(c.rcvNxt, seq+uint32(len(data))) {
		// Resent with some we have: the rest is new.
		data = data[c.rcvNxt-seq:]
		seq = c.rcvNxt
	}
	if len(data) > 0 && seqLT(c.rcvNxt, seq) && !c.peerFin {
		s.stats.TCPOutOfOrder++
		if !s.hold(c, seq, data) {
			s.stats.TCPOutOfOrderDropped++
		}
	}
	if seq == c.rcvNxt && !c.peerFin {
		n := min(len(data), len(c.rcv)-c.rcvLen)
		if n > 0 {
			c.put(0, data[:n])
			c.rcvLen += n
			c.rcvNxt += uint32(n)
			s.stats.TCPBytesIn += uint32(n)
			s.joinHeld(c)
			c.wake()
		}
		if flags&tcpFIN != 0 && n == len(data) && c.nHeld == 0 {
			c.rcvNxt++
			c.peerFin = true
			switch c.state {
			case tcpEstablished:
				c.state = tcpCloseWait
			case tcpFinWait1:
				c.state = tcpClosing
			case tcpFinWait2:
				s.enterTimeWait(c)
			}
			c.wake()
		}
	}
	if needACK {
		s.tcpSend(c, tcpACK, c.sndNxt, 0, 0)
	}
	s.tcpOutput(c)
}

// put copies data into the receive ring at off bytes past rcvNxt.
func (c *TCPConn) put(off int, data []byte) {
	i := (c.rcvHead + c.rcvLen + off) % len(c.rcv)
	m := copy(c.rcv[i:], data)
	copy(c.rcv, data[m:])
}

// hold keeps data that came past a gap, at seq, in the receive ring where it
// will go, if it is all in the window and there is a range to note it in. Its
// bytes are the same as any already held there. The caller holds s.mu.
func (s *Stack) hold(c *TCPConn, seq uint32, data []byte) bool {
	off := int(seq - c.rcvNxt)
	if off+len(data) > len(c.rcv)-c.rcvLen {
		return false // past the window
	}
	r := span{seq, seq + uint32(len(data))}
	// Where it goes in the list, and the ranges it overlaps or touches.
	i := 0
	for i < c.nHeld && seqLT(c.held[i].end, r.start) {
		i++
	}
	j := i
	for j < c.nHeld && !seqLT(r.end, c.held[j].start) {
		if seqLT(c.held[j].start, r.start) {
			r.start = c.held[j].start
		}
		if seqLT(r.end, c.held[j].end) {
			r.end = c.held[j].end
		}
		j++
	}
	if j == i && c.nHeld == maxHeld {
		return false // a new range, and no room for it
	}
	c.put(off, data)
	// Ranges i to j-1 become r.
	copy(c.held[i+1:], c.held[j:c.nHeld])
	c.nHeld += 1 - (j - i)
	c.held[i] = r
	return true
}

// joinHeld adds the held data that rcvNxt has reached to the data. The
// caller holds s.mu.
func (s *Stack) joinHeld(c *TCPConn) {
	k := 0
	for k < c.nHeld && !seqLT(c.rcvNxt, c.held[k].start) {
		if r := c.held[k]; seqLT(c.rcvNxt, r.end) {
			n := int(r.end - c.rcvNxt)
			c.rcvLen += n
			c.rcvNxt = r.end
			s.stats.TCPBytesIn += uint32(n)
		}
		k++
	}
	copy(c.held[:], c.held[k:c.nHeld])
	c.nHeld -= k
}

// enterTimeWait starts TIME-WAIT. Only Close sends our FIN, so the user is
// done, and TIME-WAIT sends no data: the rings go back to the allocator now,
// and only the slot waits.
func (s *Stack) enterTimeWait(c *TCPConn) {
	c.state = tcpTimeWait
	c.rtoAt = time.Time{}
	c.waitDone = time.Now().Add(timeWaitTime)
	s.freeRings(c)
	c.wake()
}

func (s *Stack) freeRings(c *TCPConn) {
	if c.snd != nil {
		memory.FreeSlice(s.mem, c.snd)
		memory.FreeSlice(s.mem, c.rcv)
		c.snd, c.rcv = nil, nil
		c.sndLen, c.rcvLen, c.sent, c.nHeld = 0, 0, 0, 0
	}
}

// tcpNoConn opens a connection for a SYN to a listener, or answers with a
// RST. The caller holds s.mu.
func (s *Stack) tcpNoConn(mac [6]byte, src Addr, sport, dport uint16, seq, ack uint32, flags byte, n, mss int) {
	if flags&tcpRST != 0 {
		return
	}
	if flags&(tcpSYN|tcpACK) == tcpSYN {
		for _, l := range s.tcpListeners {
			if l == nil || l.port != dport {
				continue
			}
			c, err := s.newTCPConn(dport, src, sport, mac, l.bufSize)
			if err != nil {
				// No slot or no memory: drop the SYN, so the peer sends it
				// again later, as tcp_listen_input of lwIP 2.1 tcp_in.c.
				return
			}
			c.lis = l
			c.state = tcpSynRcvd
			c.rcvNxt = seq + 1
			c.sndNxt = c.iss + 1
			c.mss = min(mss, tcpMSS)
			s.tcpSendSYN(c)
			return
		}
	}
	// RFC 9293 3.10.7.1: answer with a RST.
	r := TCPConn{lport: dport, rport: sport, raddr: src, mac: mac}
	if flags&tcpACK != 0 {
		s.tcpSend(&r, tcpRST, ack, 0, 0)
		return
	}
	r.rcvNxt = seq + uint32(n)
	if flags&tcpSYN != 0 {
		r.rcvNxt++
	}
	s.tcpSend(&r, tcpRST|tcpACK, 0, 0, 0)
}
