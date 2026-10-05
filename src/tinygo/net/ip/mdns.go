package ip

import "time"

// A Multicast DNS responder for the A record of one host name (RFC 6762). It
// does not probe for name conflicts (section 8.1).

const (
	mdnsPort       = 5353
	mdnsTTL        = 120 // host records, RFC 6762 section 10
	mdnsLegacyTTL  = 10  // unicast answers to other ports, section 6.7
	mdnsCacheFlush = 0x8000
	mdnsAnnounces  = 2 // section 8.3
	dnsTypeANY     = 255
)

// MDNSGroup is the IPv4 group of Multicast DNS.
var MDNSGroup = Addr{224, 0, 0, 251}

// ServeMDNS answers the queries for the A record of name.local with the
// address of the stack, and announces it at the start. name is one DNS label,
// for example "tinygo-node". It returns only on an error.
func (s *Stack) ServeMDNS(name string) error {
	if len(name) == 0 || len(name) > 63 {
		return ErrBadName
	}
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return ErrBadName
		}
	}
	c, err := s.ListenUDP(mdnsPort, 2048)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := s.JoinMulticast(MDNSGroup); err != nil {
		return err
	}
	defer s.LeaveMulticast(MDNSGroup)

	var in [512]byte
	var out [256]byte
	announced := 0
	next := time.Now()
	for {
		if announced < mdnsAnnounces && !time.Now().Before(next) {
			if me := s.Config().Addr; !me.IsZero() {
				n := putMDNSAnswer(out[:], name, me, 0, nil)
				if err := c.WriteTo(out[:n], MDNSGroup, mdnsPort); err != nil {
					return err
				}
				announced++
			}
			next = time.Now().Add(time.Second)
		}
		c.SetReadDeadline(time.Now().Add(time.Second))
		m, from, port, err := c.ReadFrom(in[:])
		if err == ErrDeadline {
			continue
		}
		if err != nil {
			return err
		}
		q, ok := mdnsQuestion(in[:m], name)
		me := s.Config().Addr
		if !ok || me.IsZero() {
			continue
		}
		if port == mdnsPort {
			n := putMDNSAnswer(out[:], name, me, 0, nil)
			err = c.WriteTo(out[:n], MDNSGroup, mdnsPort)
		} else {
			// A legacy resolver: a unicast answer with its ID and question.
			n := putMDNSAnswer(out[:], name, me, be16(in[0:]), q)
			err = c.WriteTo(out[:n], from, port)
		}
		if err != nil && err != ErrNoARP {
			return err
		}
	}
}

// mdnsQuestion gives the first question of the query m that asks for the A
// record of name.local.
func mdnsQuestion(m []byte, name string) ([]byte, bool) {
	if len(m) < dnsHdrLen || m[2]&0xf8 != 0 { // a response, or not QUERY
		return nil, false
	}
	off := dnsHdrLen
	for q := int(be16(m[4:])); q > 0; q-- {
		start := off
		match, next, ok := matchName(m, off, name)
		if !ok || next+4 > len(m) {
			return nil, false
		}
		typ, class := be16(m[next:]), be16(m[next+2:])&^mdnsCacheFlush
		off = next + 4
		if match && (typ == dnsTypeA || typ == dnsTypeANY) && class == dnsClassIN {
			return m[start:off], true
		}
	}
	return nil, false
}

// matchName reports whether the name at off is name.local, without case,
// and gives the offset after the name. It follows compression pointers.
func matchName(m []byte, off int, name string) (match bool, next int, ok bool) {
	labels := [2]string{name, "local"}
	i, next := 0, -1
	match = true
	for jumps := 0; jumps < 16; {
		if off >= len(m) {
			return false, 0, false
		}
		l := int(m[off])
		switch {
		case l == 0:
			if next < 0 {
				next = off + 1
			}
			return match && i == len(labels), next, true
		case l&0xc0 == 0xc0:
			if off+2 > len(m) {
				return false, 0, false
			}
			if next < 0 {
				next = off + 2
			}
			off = int(be16(m[off:]) & 0x3fff)
			jumps++
			continue
		case l&0xc0 != 0 || off+1+l > len(m):
			return false, 0, false
		}
		if i >= len(labels) || !equalFold(m[off+1:off+1+l], labels[i]) {
			match = false
		}
		i++
		off += 1 + l
	}
	return false, 0, false
}

func equalFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		x, y := b[i], s[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// putMDNSAnswer writes a response with the A record of name.local. A nil
// question makes a multicast response: ID 0, the cache-flush bit and the long
// TTL. Else it is a unicast answer to a legacy resolver (RFC 6762 section 6.7).
func putMDNSAnswer(b []byte, name string, addr Addr, id uint16, question []byte) int {
	clear(b[:dnsHdrLen])
	put16(b[0:], id)
	b[2] = 0x84 // QR and AA
	n := dnsHdrLen
	class, ttl := uint16(dnsClassIN|mdnsCacheFlush), uint32(mdnsTTL)
	if question != nil {
		put16(b[4:], 1)
		n += copy(b[n:], question)
		class, ttl = dnsClassIN, mdnsLegacyTTL
	}
	put16(b[6:], 1)
	b[n] = byte(len(name))
	n += 1 + copy(b[n+1:], name)
	b[n] = 5
	n += 1 + copy(b[n+1:], "local")
	b[n] = 0
	n++
	put16(b[n:], dnsTypeA)
	put16(b[n+2:], class)
	put32(b[n+4:], ttl)
	put16(b[n+8:], 4)
	copy(b[n+10:], addr[:])
	return n + 14
}
