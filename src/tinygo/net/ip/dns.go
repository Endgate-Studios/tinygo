package ip

import "time"

// A DNS client for A records (RFC 1035).

const (
	dnsPort    = 53
	dnsHdrLen  = 12
	dnsTypeA   = 1
	dnsClassIN = 1
	dnsTries   = 3
	dnsBufSize = 1024
	maxNameLen = 253
)

const (
	ErrNoDNS     Error = "ip: no DNS server"
	ErrBadName   Error = "ip: bad host name"
	ErrNotFound  Error = "ip: host not found"
	ErrDNSFormat Error = "ip: bad DNS answer"
)

// LookupHost gives an IPv4 address of name from the DNS server of the
// configuration.
func (s *Stack) LookupHost(name string, timeout time.Duration) (Addr, error) {
	if a, ok := parseAddr(name); ok {
		return a, nil
	}
	server := s.Config().DNS
	if server.IsZero() {
		return Addr{}, ErrNoDNS
	}
	// The buffers are in the stack, so a lookup does not allocate.
	s.dnsMu.Lock()
	defer s.dnsMu.Unlock()
	q, buf := &s.dnsQuery, &s.dnsAnswer
	n, err := putQuery(q[:], name)
	if err != nil {
		return Addr{}, err
	}
	c, err := s.ListenUDP(0, dnsBufSize)
	if err != nil {
		return Addr{}, err
	}
	defer c.Close()

	s.mu.Lock()
	s.isnSeed = s.isnSeed*1103515245 + uint32(time.Now().UnixNano())
	id := uint16(s.isnSeed >> 16)
	s.mu.Unlock()
	put16(q[0:], id)

	per := timeout / dnsTries
	for try := 0; try < dnsTries; try++ {
		if err := c.WriteTo(q[:n], server, dnsPort); err != nil {
			return Addr{}, err
		}
		end := time.Now().Add(per)
		for time.Now().Before(end) {
			m, from, port, ok, err := c.TryReadFrom(buf[:])
			if err != nil {
				return Addr{}, err
			}
			if !ok {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if from != server || port != dnsPort || m < dnsHdrLen || be16(buf[0:]) != id {
				continue
			}
			return parseAnswer(buf[:m])
		}
	}
	return Addr{}, ErrTimeout
}

// putQuery writes a recursive query for the A record of name and gives its
// length. The ID stays 0.
func putQuery(b []byte, name string) (int, error) {
	if len(name) > 0 && name[len(name)-1] == '.' {
		name = name[:len(name)-1]
	}
	if len(name) == 0 || len(name) > maxNameLen {
		return 0, ErrBadName
	}
	clear(b[:dnsHdrLen])
	b[2] = 0x01 // RD
	put16(b[4:], 1)
	n := dnsHdrLen
	for start := 0; start <= len(name); {
		end := start
		for end < len(name) && name[end] != '.' {
			end++
		}
		label := end - start
		if label == 0 || label > 63 {
			return 0, ErrBadName
		}
		b[n] = byte(label)
		n += 1 + copy(b[n+1:], name[start:end])
		start = end + 1
	}
	b[n] = 0
	put16(b[n+1:], dnsTypeA)
	put16(b[n+3:], dnsClassIN)
	return n + 5, nil
}

// parseAnswer gives the first A record of the answer section.
func parseAnswer(m []byte) (Addr, error) {
	if m[2]&0x80 == 0 || m[2]&0x02 != 0 { // not a response, or truncated
		return Addr{}, ErrDNSFormat
	}
	if rcode := m[3] & 0x0f; rcode == 3 {
		return Addr{}, ErrNotFound
	} else if rcode != 0 {
		return Addr{}, ErrDNSFormat
	}
	answers := int(be16(m[6:]))
	off := dnsHdrLen
	for q := int(be16(m[4:])); q > 0; q-- {
		var ok bool
		if off, ok = skipName(m, off); !ok || off+4 > len(m) {
			return Addr{}, ErrDNSFormat
		}
		off += 4
	}
	for ; answers > 0; answers-- {
		var ok bool
		if off, ok = skipName(m, off); !ok || off+10 > len(m) {
			return Addr{}, ErrDNSFormat
		}
		typ, class, rdLen := be16(m[off:]), be16(m[off+2:]), int(be16(m[off+8:]))
		off += 10
		if off+rdLen > len(m) {
			return Addr{}, ErrDNSFormat
		}
		// A CNAME comes before the A record of its target, so skip it.
		if typ == dnsTypeA && class == dnsClassIN && rdLen == 4 {
			return Addr(m[off : off+4]), nil
		}
		off += rdLen
	}
	return Addr{}, ErrNotFound
}

// skipName gives the offset after the name at off, which can end in a
// compression pointer.
func skipName(m []byte, off int) (int, bool) {
	for n := 0; n < 128 && off < len(m); n++ {
		l := int(m[off])
		switch {
		case l == 0:
			return off + 1, true
		case l&0xc0 == 0xc0:
			return off + 2, off+2 <= len(m)
		case l&0xc0 != 0:
			return 0, false
		}
		off += 1 + l
	}
	return 0, false
}

// parseAddr parses a dotted decimal address.
func parseAddr(s string) (Addr, bool) {
	var a Addr
	part, digits := 0, 0
	v := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			if digits == 0 || part > 3 {
				return Addr{}, false
			}
			a[part] = byte(v)
			part++
			v, digits = 0, 0
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return Addr{}, false
		}
		v = v*10 + int(s[i]-'0')
		digits++
		if v > 255 {
			return Addr{}, false
		}
	}
	return a, part == 4
}
