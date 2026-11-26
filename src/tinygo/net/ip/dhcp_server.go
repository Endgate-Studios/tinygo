package ip

import "time"

// The DHCP server of RFC 2131 for a soft AP. It gives each client MAC one
// address of a pool and keeps no state over a restart.

const (
	dhcpDecline = 4
	dhcpRelease = 7

	// MaxDHCPClients is the largest pool of ServeDHCP.
	MaxDHCPClients = 8

	// ErrDHCPPool is a pool that ServeDHCP cannot use.
	ErrDHCPPool Error = "ip: bad DHCP pool"
)

type dhcpPool struct {
	first Addr
	n     int
	lease time.Duration
	mac   [MaxDHCPClients][6]byte
	used  [MaxDHCPClients]bool
	in    [576]byte
	out   [dhcpFixedLen + 40]byte
}

// ServeDHCP answers DHCP requests with the addresses first to first+n-1, at
// most MaxDHCPClients. The address of the stack is the server, the router and
// the DNS server. It returns only when the socket fails, so run it in a
// goroutine. Set the address of the stack first, with SetConfig.
func (s *Stack) ServeDHCP(first Addr, n int, lease time.Duration) error {
	if n < 1 || n > MaxDHCPClients || int(first[3])+n > 255 {
		return ErrDHCPPool
	}
	c, err := s.ListenUDP(dhcpServerPort, 1024)
	if err != nil {
		return err
	}
	d := &dhcpPool{first: first, n: n, lease: lease}
	for {
		m, _, _, err := c.ReadFrom(d.in[:])
		if err != nil {
			return err
		}
		if k := d.handle(s, d.in[:m]); k > 0 {
			c.WriteTo(d.out[:k], Broadcast, dhcpClientPort)
		}
	}
}

// handle parses one client message and builds the reply in d.out. It returns
// the length of the reply, or 0 when there is no reply.
func (d *dhcpPool) handle(s *Stack, p []byte) int {
	if len(p) < dhcpFixedLen || p[0] != 1 || p[1] != 1 || p[2] != 6 || be32(p[236:]) != 0x63825363 {
		return 0
	}
	mac := [6]byte(p[28:34])
	var msg byte
	var requested, server Addr
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
			return 0
		}
		v := o[2 : 2+int(o[1])]
		switch {
		case code == optMessageType && len(v) == 1:
			msg = v[0]
		case code == optRequestedIP && len(v) == 4:
			requested = Addr(v)
		case code == optServerID && len(v) == 4:
			server = Addr(v)
		}
		o = o[2+len(v):]
	}
	me := s.Config().Addr
	slot := d.find(mac)
	switch msg {
	case dhcpDiscover:
		if slot < 0 {
			if slot = d.take(mac); slot < 0 {
				return 0 // the pool is full
			}
		}
		return d.reply(s, p, dhcpOffer, d.addr(slot))
	case dhcpRequest:
		if !server.IsZero() && server != me {
			if slot >= 0 {
				d.used[slot] = false // the client chose another server
			}
			return 0
		}
		if requested.IsZero() {
			requested = Addr(p[12:16]) // ciaddr, when the client renews
		}
		if slot < 0 || requested != d.addr(slot) {
			return d.reply(s, p, dhcpNak, Addr{})
		}
		return d.reply(s, p, dhcpAck, d.addr(slot))
	case dhcpRelease, dhcpDecline:
		if slot >= 0 {
			d.used[slot] = false
		}
	}
	return 0
}

func (d *dhcpPool) addr(slot int) Addr {
	a := d.first
	a[3] += byte(slot)
	return a
}

func (d *dhcpPool) find(mac [6]byte) int {
	for i := range d.n {
		if d.used[i] && d.mac[i] == mac {
			return i
		}
	}
	return -1
}

func (d *dhcpPool) take(mac [6]byte) int {
	for i := range d.n {
		if !d.used[i] {
			d.used[i], d.mac[i] = true, mac
			return i
		}
	}
	return -1
}

// reply builds a BOOTREPLY for the request p.
func (d *dhcpPool) reply(s *Stack, p []byte, msg byte, yiaddr Addr) int {
	cfg := s.Config()
	b := d.out[:]
	clear(b)
	b[0], b[1], b[2] = 2, 1, 6 // BOOTREPLY, Ethernet, address length
	copy(b[4:8], p[4:8])       // xid
	copy(b[10:12], p[10:12])   // flags
	copy(b[16:20], yiaddr[:])
	copy(b[28:44], p[28:44]) // chaddr
	put32(b[236:], 0x63825363)
	o := b[dhcpFixedLen:dhcpFixedLen]
	o = append(o, optMessageType, 1, msg)
	o = append(o, optServerID, 4, cfg.Addr[0], cfg.Addr[1], cfg.Addr[2], cfg.Addr[3])
	if msg != dhcpNak {
		secs := uint32(d.lease / time.Second)
		o = append(o, optLeaseTime, 4, byte(secs>>24), byte(secs>>16), byte(secs>>8), byte(secs))
		o = append(o, optSubnetMask, 4, cfg.Mask[0], cfg.Mask[1], cfg.Mask[2], cfg.Mask[3])
		o = append(o, optRouter, 4, cfg.Addr[0], cfg.Addr[1], cfg.Addr[2], cfg.Addr[3])
		o = append(o, optDNS, 4, cfg.Addr[0], cfg.Addr[1], cfg.Addr[2], cfg.Addr[3])
	}
	o = append(o, optEnd)
	return dhcpFixedLen + len(o)
}
