package ip

import "time"

// The DHCP client of RFC 2131 with the options of RFC 2132.

const (
	dhcpServerPort = 67
	dhcpClientPort = 68
	dhcpFixedLen   = 240 // BOOTP fields and the magic cookie
	dhcpWait       = 2 * time.Second
	dhcpPoll       = 10 * time.Millisecond

	dhcpDiscover = 1
	dhcpOffer    = 2
	dhcpRequest  = 3
	dhcpAck      = 5
	dhcpNak      = 6

	optSubnetMask  = 1
	optRouter      = 3
	optDNS         = 6
	optRequestedIP = 50
	optLeaseTime   = 51
	optMessageType = 53
	optServerID    = 54
	optParamList   = 55
	optEnd         = 255
)

type dhcpState uint8

const (
	dhcpSelecting  dhcpState = iota // DISCOVER sent, waiting for an OFFER
	dhcpRequesting                  // REQUEST sent, waiting for the ACK
	dhcpBound
	dhcpRestart // NAK received
)

type dhcpClient struct {
	active bool
	state  dhcpState
	xid    uint32
	offer  Addr
	server Addr
	cfg    Config
	lease  time.Duration
	start  time.Time // of the lease
	renew  bool      // the goroutine of renewLoop runs
	buf    [dhcpFixedLen + 32]byte
}

// dhcpMinRetry is the shortest wait before a REQUEST is sent again in the
// RENEWING and REBINDING states (RFC 2131 4.4.5).
const dhcpMinRetry = 60 * time.Second

// DHCP gets an address with DHCP and sets it as the configuration of the
// stack. It tries until timeout.
func (s *Stack) DHCP(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	s.mu.Lock()
	d := &s.dhcp
	d.active, d.lease = true, 0
	m := s.mac
	d.xid = uint32(time.Now().UnixNano()) ^ be32(m[2:6])
	s.cfg = Config{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		d.active = false
		s.mu.Unlock()
	}()

	for time.Now().Before(deadline) {
		s.mu.Lock()
		d.state = dhcpSelecting
		err := s.sendDHCP(dhcpDiscover, Addr{}, Broadcast)
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if !s.waitDHCP(dhcpRequesting, deadline) {
			continue // no OFFER: start again
		}
		s.mu.Lock()
		err = s.sendDHCP(dhcpRequest, Addr{}, Broadcast)
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if s.waitDHCP(dhcpBound, deadline) {
			s.mu.Lock()
			s.cfg = d.cfg
			start := !d.renew && d.lease > 0
			d.renew = d.renew || start
			s.mu.Unlock()
			if start {
				go s.renewLoop()
			}
			return nil
		}
	}
	return ErrDHCPFailed
}

// waitDHCP waits until input moves the client to state, or dhcpWait passes.
func (s *Stack) waitDHCP(state dhcpState, deadline time.Time) bool {
	end := time.Now().Add(dhcpWait)
	if deadline.Before(end) {
		end = deadline
	}
	for time.Now().Before(end) {
		time.Sleep(dhcpPoll)
		s.mu.Lock()
		got := s.dhcp.state
		s.mu.Unlock()
		if got == state {
			return true
		}
		if got == dhcpRestart {
			return false
		}
	}
	return false
}

// Lease gives the lease time of the last DHCP ACK.
func (s *Stack) Lease() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dhcp.lease
}

// renewLoop keeps the lease: RENEWING at T1, REBINDING at T2, and a new
// DHCP when the lease ends (RFC 2131 4.4.5). It sleeps between the steps,
// which allocates nothing.
func (s *Stack) renewLoop() {
	for {
		s.mu.Lock()
		start, lease := s.dhcp.start, s.dhcp.lease
		if lease == 0 {
			s.dhcp.renew = false // no lease time: the address does not end
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		t1 := start.Add(lease / 2)
		t2 := start.Add(lease / 8 * 7)
		end := start.Add(lease)
		time.Sleep(time.Until(t1))
		if s.renewLease(false, t2) || s.renewLease(true, end) {
			continue
		}
		// The lease ended. DHCP does not start a second loop, because
		// d.renew is still true.
		for s.DHCP(time.Minute) != nil {
		}
	}
}

// renewLease sends REQUESTs for the current address until an ACK comes or
// until. It unicasts to the server, or broadcasts when rebind is true.
func (s *Stack) renewLease(rebind bool, until time.Time) bool {
	for time.Now().Before(until) {
		s.mu.Lock()
		d := &s.dhcp
		d.active, d.state, d.offer = true, dhcpRequesting, s.cfg.Addr
		dst := d.server
		if rebind {
			dst = Broadcast
		}
		err := s.sendDHCP(dhcpRequest, s.cfg.Addr, dst)
		s.mu.Unlock()
		wait := max(time.Until(until)/2, dhcpMinRetry)
		ok := err == nil && s.waitDHCP(dhcpBound, time.Now().Add(min(wait, time.Until(until))))
		s.mu.Lock()
		d.active = false
		if ok {
			s.cfg = d.cfg
			s.dhcp.renew = true
		}
		s.mu.Unlock()
		if ok {
			return true
		}
		if err != nil {
			time.Sleep(min(dhcpMinRetry, time.Until(until)))
		}
	}
	return false
}

// sendDHCP sends a DISCOVER or a REQUEST from ciaddr to dst. A zero ciaddr is
// the SELECTING state, which asks for the offer. The caller holds s.mu.
func (s *Stack) sendDHCP(msg byte, ciaddr, dst Addr) error {
	d := &s.dhcp
	b := d.buf[:]
	clear(b)
	b[0], b[1], b[2] = 1, 1, 6 // BOOTREQUEST, Ethernet, address length
	put32(b[4:], d.xid)
	if ciaddr.IsZero() {
		put16(b[10:], 0x8000) // broadcast reply, as the client has no address
	}
	copy(b[12:16], ciaddr[:])
	copy(b[28:34], s.mac[:])
	put32(b[236:], 0x63825363) // magic cookie
	o := b[dhcpFixedLen:dhcpFixedLen]
	o = append(o, optMessageType, 1, msg)
	if msg == dhcpRequest && ciaddr.IsZero() {
		o = append(o, optRequestedIP, 4, d.offer[0], d.offer[1], d.offer[2], d.offer[3])
		o = append(o, optServerID, 4, d.server[0], d.server[1], d.server[2], d.server[3])
	}
	o = append(o, optParamList, 3, optSubnetMask, optRouter, optDNS, optEnd)
	return s.sendUDPLocked(ciaddr, dhcpClientPort, dst, dhcpServerPort, b[:dhcpFixedLen+len(o)])
}

// input handles a reply from a server. The caller holds s.mu.
func (d *dhcpClient) input(s *Stack, p []byte) {
	if len(p) < dhcpFixedLen || p[0] != 2 || be32(p[4:]) != d.xid ||
		[6]byte(p[28:34]) != s.mac || be32(p[236:]) != 0x63825363 {
		return
	}
	var msg byte
	var cfg Config
	var server Addr
	var lease uint32
	for o := p[dhcpFixedLen:]; len(o) > 0; {
		code := o[0]
		if code == optEnd {
			break
		}
		if code == 0 { // pad
			o = o[1:]
			continue
		}
		if len(o) < 2 || len(o) < 2+int(o[1]) {
			return
		}
		v := o[2 : 2+int(o[1])]
		switch {
		case code == optMessageType && len(v) == 1:
			msg = v[0]
		case code == optSubnetMask && len(v) == 4:
			cfg.Mask = Addr(v)
		case code == optRouter && len(v) >= 4:
			cfg.Gateway = Addr(v[:4])
		case code == optDNS && len(v) >= 4:
			cfg.DNS = Addr(v[:4])
		case code == optServerID && len(v) == 4:
			server = Addr(v)
		case code == optLeaseTime && len(v) == 4:
			lease = be32(v)
		}
		o = o[2+len(v):]
	}
	cfg.Addr = Addr(p[16:20]) // yiaddr
	switch {
	case msg == dhcpOffer && d.state == dhcpSelecting:
		d.offer, d.server = cfg.Addr, server
		d.state = dhcpRequesting
	case msg == dhcpAck && d.state == dhcpRequesting && cfg.Addr == d.offer:
		d.cfg = cfg
		d.lease = time.Duration(lease) * time.Second
		d.start = time.Now()
		d.state = dhcpBound
	case msg == dhcpNak && d.state == dhcpRequesting:
		d.state = dhcpRestart
	}
}
