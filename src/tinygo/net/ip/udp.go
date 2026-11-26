package ip

import (
	"time"

	"runtime/memory"
)

// UDP of RFC 768.

const (
	maxSockets    = 8
	recordHeader  = 8 // length, source address, source port
	ephemeralBase = 49152
)

// UDPConn is a UDP socket on one local port. Do not use it after Close: the
// stack reuses it for a new socket.
type UDPConn struct {
	s     *Stack
	port  uint16
	ring  []byte // from the allocator of the stack
	head  int    // first byte of the oldest record
	used  int
	ready chan struct{}
	shut  bool

	readDeadline time.Time

	// Dropped counts the datagrams that did not fit in the ring.
	Dropped uint32
}

// ListenUDP opens a socket on port, or on a free port when port is 0. The
// socket keeps up to bufSize bytes of received datagrams, each with 8 bytes
// of header.
func (s *Stack) ListenUDP(port uint16, bufSize int) (*UDPConn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if port == 0 {
		for p := uint16(ephemeralBase); p != 0; p++ {
			if s.socket(p) == nil {
				port = p
				break
			}
		}
	}
	if s.socket(port) != nil {
		return nil, ErrPortInUse
	}
	slot := -1
	for i, c := range s.udp {
		if c == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		return nil, ErrNoSockets
	}
	ring := memory.MakeSlice[byte](s.mem, bufSize)
	if ring == nil {
		return nil, ErrNoMemory
	}
	// The socket objects and their channels are reused, as for TCP.
	c := &s.udpStore[slot]
	ready := c.ready
	if ready == nil {
		ready = make(chan struct{}, 1)
	}
	drain(ready)
	*c = UDPConn{s: s, port: port, ring: ring, ready: ready}
	s.udp[slot] = c
	return c, nil
}

func (s *Stack) socket(port uint16) *UDPConn {
	for _, c := range s.udp {
		if c != nil && c.port == port {
			return c
		}
	}
	return nil
}

// Port gives the local port.
func (c *UDPConn) Port() uint16 { return c.port }

// Close frees the socket and its buffer. A blocked ReadFrom returns ErrClosed.
func (c *UDPConn) Close() error {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.shut {
		return ErrClosed
	}
	c.shut = true
	for i, u := range s.udp {
		if u == c {
			s.udp[i] = nil
		}
	}
	memory.FreeSlice(s.mem, c.ring)
	c.ring = nil
	select {
	case c.ready <- struct{}{}:
	default:
	}
	return nil
}

// SetReadDeadline sets when ReadFrom gives up with ErrDeadline. Zero waits
// forever.
func (c *UDPConn) SetReadDeadline(t time.Time) {
	c.s.mu.Lock()
	c.readDeadline = t
	c.s.mu.Unlock()
}

// ReadFrom waits for a datagram and copies it into b. A datagram larger than
// b is cut.
func (c *UDPConn) ReadFrom(b []byte) (n int, from Addr, port uint16, err error) {
	for {
		n, from, port, ok, err := c.TryReadFrom(b)
		if ok || err != nil {
			return n, from, port, err
		}
		c.s.mu.Lock()
		deadline := c.readDeadline
		c.s.mu.Unlock()
		if !wait(c.ready, deadline) {
			return 0, Addr{}, 0, ErrDeadline
		}
	}
}

// TryReadFrom is ReadFrom without a wait. ok is false when no datagram is
// there.
func (c *UDPConn) TryReadFrom(b []byte) (n int, from Addr, port uint16, ok bool, err error) {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.shut {
		return 0, Addr{}, 0, false, ErrClosed
	}
	if c.used == 0 {
		return 0, Addr{}, 0, false, nil
	}
	var h [recordHeader]byte
	c.ringRead(h[:], 0)
	size := int(be16(h[0:]))
	from, port = Addr(h[2:6]), be16(h[6:])
	n = min(size, len(b))
	c.ringRead(b[:n], recordHeader)
	c.head = (c.head + recordHeader + size) % len(c.ring)
	c.used -= recordHeader + size
	return n, from, port, true, nil
}

// ringRead copies from the ring at offset off after the head.
func (c *UDPConn) ringRead(dst []byte, off int) {
	i := (c.head + off) % len(c.ring)
	n := copy(dst, c.ring[i:])
	copy(dst[n:], c.ring)
}

func (c *UDPConn) ringWrite(src []byte, off int) {
	i := (c.head + c.used + off) % len(c.ring)
	n := copy(c.ring[i:], src)
	copy(c.ring, src[n:])
}

// push adds a datagram to the ring. The caller holds s.mu.
func (c *UDPConn) push(from Addr, port uint16, data []byte) bool {
	need := recordHeader + len(data)
	if need > len(c.ring)-c.used {
		c.Dropped++
		return false
	}
	var h [recordHeader]byte
	put16(h[0:], uint16(len(data)))
	copy(h[2:6], from[:])
	put16(h[6:], port)
	c.ringWrite(h[:], 0)
	c.ringWrite(data, recordHeader)
	c.used += need
	select {
	case c.ready <- struct{}{}:
	default:
	}
	return true
}

// WriteTo sends b to the address and port. It waits for ARP if the next hop
// is not known.
func (c *UDPConn) WriteTo(b []byte, to Addr, port uint16) error {
	s := c.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.shut {
		return ErrClosed
	}
	if s.cfg.Addr.IsZero() && to != Broadcast {
		return ErrNoAddress
	}
	return s.sendUDPLocked(s.cfg.Addr, c.port, to, port, b)
}

// sendUDPLocked resolves the next hop, then writes the datagram into s.tx
// and sends it. The caller holds s.mu.
func (s *Stack) sendUDPLocked(src Addr, sport uint16, dst Addr, dport uint16, b []byte) error {
	if ipLen+udpLen+len(b) > mtu {
		return ErrTooLarge
	}
	mac, err := s.resolve(dst)
	if err != nil {
		return err
	}
	u := s.tx[ethLen+ipLen : ethLen+ipLen+udpLen+len(b)]
	put16(u[0:], sport)
	put16(u[2:], dport)
	put16(u[4:], uint16(len(u)))
	u[6], u[7] = 0, 0
	copy(u[udpLen:], b)
	sum := fold(checksum(pseudoHeader(src, dst, protoUDP, len(u)), u))
	if sum == 0 {
		sum = 0xffff // 0 means no checksum
	}
	put16(u[6:], sum)
	s.putIPv4Header(src, dst, protoUDP, len(u))
	return s.sendEthernet(mac, etherIPv4, ipLen+len(u))
}

func pseudoHeader(src, dst Addr, proto byte, n int) uint32 {
	sum := checksum(0, src[:])
	sum = checksum(sum, dst[:])
	return sum + uint32(proto) + uint32(n)
}

func (s *Stack) inputUDP(src, dst Addr, p []byte) {
	if len(p) < udpLen {
		s.stats.Dropped++
		return
	}
	n := int(be16(p[4:]))
	if n < udpLen || n > len(p) {
		s.stats.Dropped++
		return
	}
	p = p[:n]
	if be16(p[6:]) != 0 && fold(checksum(pseudoHeader(src, dst, protoUDP, n), p)) != 0 {
		s.stats.Dropped++
		return
	}
	sport, dport := be16(p[0:]), be16(p[2:])
	if dport == dhcpClientPort && s.dhcp.active {
		s.dhcp.input(s, p[udpLen:])
		return
	}
	if s.cfg.Addr.IsZero() {
		return
	}
	c := s.socket(dport)
	if c == nil {
		return
	}
	if c.push(src, sport, p[udpLen:]) {
		s.stats.UDPDelivered++
	} else {
		s.stats.UDPDropped++
	}
}
