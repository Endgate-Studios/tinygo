// Package ip is a small IPv4 stack over an Ethernet link: ARP, IPv4, ICMP echo,
// UDP with broadcast and link-local multicast, TCP, a DHCP client and server,
// a DNS client and an mDNS responder. It does not allocate for each packet.
// Received frames are parsed in place, replies use one transmit buffer, and
// each socket has ring buffers from an injected allocator.
package ip

import (
	"sync"
	"time"

	"runtime/memory"
)

// Link sends Ethernet frames. Give the received frames to Stack.Input.
type Link interface {
	MAC() [6]byte
	Send(frame []byte) error
}

// Memory is where the stack allocates the socket buffers.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Addr is an IPv4 address.
type Addr [4]byte

// Broadcast is the limited broadcast address.
var Broadcast = Addr{255, 255, 255, 255}

func (a Addr) IsZero() bool { return a == Addr{} }

// String formats a as a dotted decimal address.
func (a Addr) String() string {
	var b [15]byte
	n := 0
	for i, v := range a {
		if i > 0 {
			b[n] = '.'
			n++
		}
		if v >= 100 {
			b[n] = '0' + v/100
			n++
		}
		if v >= 10 {
			b[n] = '0' + v/10%10
			n++
		}
		b[n] = '0' + v%10
		n++
	}
	return string(b[:n])
}

// Config is the address configuration of the stack.
type Config struct {
	Addr    Addr
	Mask    Addr
	Gateway Addr
	DNS     Addr
}

// Stats counts what the stack did with the received frames.
type Stats struct {
	Frames, Dropped, ARPReplies, EchoReplies, UDPDelivered, UDPDropped uint32
	TCPSegments, TCPBytesIn                                            uint32
	// TCPOutOfOrder counts the data segments after a gap, which the stack drops.
	TCPOutOfOrder uint32
}

const (
	ethLen     = 14
	ipLen      = 20
	udpLen     = 8
	mtu        = 1500
	maxFrame   = ethLen + mtu
	etherIPv4  = 0x0800
	etherARP   = 0x0806
	protoICMP  = 1
	protoUDP   = 17
	defaultTTL = 64
	maxGroups  = 4
)

// Stack is an IPv4 host on one link. Its methods are safe for concurrent use.
type Stack struct {
	link Link
	mem  Memory
	mac  [6]byte

	mu       sync.Mutex
	cfg      Config
	groups   [maxGroups]Addr // joined multicast groups
	arp      arpCache
	tx       [maxFrame]byte
	ipID     uint16
	udp      [maxSockets]*UDPConn
	udpStore [maxSockets]UDPConn
	dhcp     dhcpClient
	stats    Stats

	tcp          [maxTCPConns]*TCPConn
	tcpStore     [maxTCPConns]TCPConn
	tcpListeners [maxSockets]*TCPListener
	tcpTimer     bool
	tcpPort      uint16
	isnSeed      uint32

	dnsMu     sync.Mutex // one lookup at a time uses the buffers
	dnsQuery  [dnsHdrLen + maxNameLen + 2 + 4]byte
	dnsAnswer [512]byte
}

// New makes a stack on link. mem holds the socket buffers.
func New(link Link, mem Memory) *Stack {
	return &Stack{link: link, mem: mem, mac: link.MAC()}
}

// Config gives the current address configuration.
func (s *Stack) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetConfig sets a static address configuration.
func (s *Stack) SetConfig(c Config) {
	s.mu.Lock()
	s.cfg = c
	s.mu.Unlock()
}

// Stats gives the counters.
func (s *Stack) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// Input processes one received Ethernet frame. The stack does not keep frame
// after it returns.
func (s *Stack) Input(frame []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Frames++
	if len(frame) < ethLen {
		s.stats.Dropped++
		return
	}
	dst := [6]byte(frame[0:6])
	if dst != s.mac && dst != broadcastMAC && !s.joinedMAC(dst) {
		return // frames for other hosts and other groups
	}
	switch be16(frame[12:]) {
	case etherARP:
		s.inputARP(frame[ethLen:])
	case etherIPv4:
		s.inputIPv4([6]byte(frame[6:12]), frame[ethLen:])
	}
}

var broadcastMAC = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// sendEthernet sends s.tx[:ethLen+n] after it writes the Ethernet header.
// The caller holds s.mu and has written the payload at s.tx[ethLen:].
func (s *Stack) sendEthernet(dst [6]byte, etherType uint16, n int) error {
	copy(s.tx[0:6], dst[:])
	copy(s.tx[6:12], s.mac[:])
	put16(s.tx[12:], etherType)
	return s.link.Send(s.tx[:ethLen+n])
}

// nextHop gives the address to resolve for dst: dst itself on the subnet,
// else the gateway.
func (s *Stack) nextHop(dst Addr) Addr {
	c := s.cfg
	for i := range dst {
		if dst[i]&c.Mask[i] != c.Addr[i]&c.Mask[i] {
			return c.Gateway
		}
	}
	return dst
}

// isBroadcast reports whether dst is the limited or the subnet broadcast.
func (s *Stack) isBroadcast(dst Addr) bool {
	if dst == Broadcast {
		return true
	}
	c := s.cfg
	if c.Mask.IsZero() {
		return false
	}
	for i := range dst {
		if dst[i]|c.Mask[i] != 0xff || dst[i]&c.Mask[i] != c.Addr[i]&c.Mask[i] {
			return false
		}
	}
	return true
}

// JoinMulticast receives the UDP datagrams to group, a multicast address. It
// sends no IGMP report, so use it for the link-local groups 224.0.0.0/24,
// which switches send to all ports (RFC 4541 section 2.1.2).
func (s *Stack) JoinMulticast(group Addr) error {
	if !isMulticast(group) {
		return ErrNotMulticast
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	free := -1
	for i, g := range s.groups {
		if g == group {
			return nil
		}
		if g.IsZero() && free < 0 {
			free = i
		}
	}
	if free < 0 {
		return ErrGroups
	}
	s.groups[free] = group
	return nil
}

// LeaveMulticast stops the reception of group.
func (s *Stack) LeaveMulticast(group Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.groups {
		if g == group {
			s.groups[i] = Addr{}
		}
	}
}

func isMulticast(a Addr) bool { return a[0]&0xf0 == 0xe0 }

// multicastMAC maps a group to its Ethernet address (RFC 1112 section 6.4).
func multicastMAC(a Addr) [6]byte {
	return [6]byte{0x01, 0x00, 0x5e, a[1] & 0x7f, a[2], a[3]}
}

// joined reports whether the stack receives group. The caller holds s.mu.
func (s *Stack) joined(group Addr) bool {
	for _, g := range s.groups {
		if g == group && !g.IsZero() {
			return true
		}
	}
	return false
}

// joinedMAC reports whether mac is the address of a joined group. The caller
// holds s.mu.
func (s *Stack) joinedMAC(mac [6]byte) bool {
	if mac[0]&1 == 0 {
		return false
	}
	for _, g := range s.groups {
		if !g.IsZero() && multicastMAC(g) == mac {
			return true
		}
	}
	return false
}

// resolve gives the MAC of dst. It sends ARP requests and waits for a reply.
// The caller holds s.mu, and resolve releases it while it waits. So resolve
// before you write a packet into s.tx, and never call it from Input.
func (s *Stack) resolve(dst Addr) ([6]byte, error) {
	if s.isBroadcast(dst) {
		return broadcastMAC, nil
	}
	if isMulticast(dst) {
		return multicastMAC(dst), nil
	}
	hop := s.nextHop(dst)
	if hop.IsZero() {
		return [6]byte{}, ErrNoRoute
	}
	for try := 0; try < arpTries; try++ {
		if mac, ok := s.arp.lookup(hop); ok {
			return mac, nil
		}
		s.sendARP(arpRequest, [6]byte{}, hop)
		// time.Sleep allocates nothing, unlike time.After.
		for waited := time.Duration(0); waited < arpWait; waited += arpPoll {
			s.mu.Unlock()
			time.Sleep(arpPoll)
			s.mu.Lock()
			if _, ok := s.arp.lookup(hop); ok {
				break
			}
		}
	}
	if mac, ok := s.arp.lookup(hop); ok {
		return mac, nil
	}
	return [6]byte{}, ErrNoARP
}

// Error is an error of the stack.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNoRoute      Error = "ip: no route to the address"
	ErrNoARP        Error = "ip: no ARP reply"
	ErrTooLarge     Error = "ip: datagram too large"
	ErrPortInUse    Error = "ip: port in use"
	ErrNoSockets    Error = "ip: no free socket"
	ErrClosed       Error = "ip: socket closed"
	ErrNoMemory     Error = "ip: no memory for the socket buffer"
	ErrNoAddress    Error = "ip: no address configured"
	ErrDHCPFailed   Error = "ip: no DHCP lease"
	ErrDeadline     Error = "ip: deadline exceeded"
	ErrNotMulticast Error = "ip: not a multicast address"
	ErrGroups       Error = "ip: too many multicast groups"
)

// deadlinePoll is the step of a wait with a deadline. time.Sleep allocates
// nothing, unlike a timer.
const deadlinePoll = 5 * time.Millisecond

// wait waits for a signal on ch. A zero deadline waits forever. It reports
// false when the deadline passed.
func wait(ch chan struct{}, deadline time.Time) bool {
	if deadline.IsZero() {
		<-ch
		return true
	}
	for {
		select {
		case <-ch:
			return true
		default:
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(min(deadlinePoll, time.Until(deadline)))
	}
}

func be16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }

func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func put16(b []byte, v uint16) { b[0], b[1] = byte(v>>8), byte(v) }

func put32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
}

// checksum adds b to the one's complement sum (RFC 1071).
func checksum(sum uint32, b []byte) uint32 {
	for len(b) >= 2 {
		sum += uint32(b[0])<<8 | uint32(b[1])
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	return sum
}

func fold(sum uint32) uint16 {
	for sum > 0xffff {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}
