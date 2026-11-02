package ip

import (
	"time"

	"runtime/memory"
)

// A one-shot Multicast DNS lookup of the instances of a DNS-SD service type
// (RFC 6762 section 5.1, RFC 6763 section 4). It sends the queries from
// another port than 5353, so the responders answer it with unicast (section
// 6.7), and it needs no multicast group.

const (
	mdnsLookupRing = 4096
	mdnsFollowUp   = 150 * time.Millisecond // before a query for missing records
)

// ServiceInstance is an instance of a service found by LookupMDNS.
type ServiceInstance struct {
	Addr    Addr
	Port    uint16
	name    [63]byte
	nameLen uint8
	host    [63]byte
	hostLen uint8
}

// Name gives the instance name, for example "Living Room".
func (e *ServiceInstance) Name() string { return string(e.name[:e.nameLen]) }

// Host gives the host label of the instance, without ".local".
func (e *ServiceInstance) Host() string { return string(e.host[:e.hostLen]) }

func (e *ServiceInstance) resolved() bool { return e.Port != 0 && !e.Addr.IsZero() }

// LookupMDNS finds the instances of service, for example
// "_tinsel-surround._tcp", on the link and resolves them to an IPv4 address
// and a port. It asks again after 1, 3, 7... seconds, and asks for the SRV
// and A records the answers lack. It returns when out is full of resolved
// instances or after timeout, with the number of resolved instances at the
// start of out, or ErrNotFound for none.
func (s *Stack) LookupMDNS(service string, out []ServiceInstance, timeout time.Duration) (int, error) {
	var svc [mdnsMaxSvcName]byte
	svcN, err := putServiceType(svc[:], service)
	if err != nil {
		return 0, err
	}
	if len(out) == 0 {
		return 0, nil
	}
	c, err := s.ListenUDP(0, mdnsLookupRing)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	in := memory.MakeSlice[byte](s.mem, mdnsMaxMsg)
	if in == nil {
		return 0, ErrNoMemory
	}
	defer memory.FreeSlice(s.mem, in)
	var q [512]byte

	s.mu.Lock()
	s.isnSeed = s.isnSeed*1103515245 + uint32(time.Now().UnixNano())
	id := uint16(s.isnSeed >> 16)
	s.mu.Unlock()

	clear(out)
	found := 0
	now := time.Now()
	deadline, next, interval := now.Add(timeout), now, time.Second
	for {
		now = time.Now()
		if !now.Before(next) {
			n := putLookupQuery(q[:], id, svc[:svcN], out[:found])
			if err := c.WriteTo(q[:n], MDNSGroup, mdnsPort); err != nil {
				return 0, err
			}
			next, interval = now.Add(interval), interval*2
		}
		resolved := 0
		for i := range out[:found] {
			if out[i].resolved() {
				resolved++
			}
		}
		if resolved == len(out) || !now.Before(deadline) {
			break
		}
		m, _, port, ok, err := c.TryReadFrom(in)
		if err != nil {
			return 0, err
		}
		if !ok {
			wait(c.ready, minTime(next, deadline))
			continue
		}
		if port != mdnsPort {
			continue
		}
		found = parseLookup(in[:m], svc[:svcN], out, found)
		for i := range out[:found] {
			if !out[i].resolved() {
				next = minTime(next, now.Add(mdnsFollowUp))
				break
			}
		}
	}
	// The resolved instances go first.
	n := 0
	for i := range out[:found] {
		if out[i].resolved() {
			out[n], out[i] = out[i], out[n]
			n++
		}
	}
	if n == 0 {
		return 0, ErrNotFound
	}
	return n, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// putLookupQuery writes a query for the PTR records of svc.local, with a
// question for the SRV record of each instance of found without a port and
// for the A record of each one without an address. It gives the length.
func putLookupQuery(b []byte, id uint16, svc []byte, found []ServiceInstance) int {
	clear(b[:dnsHdrLen])
	put16(b[0:], id)
	n := dnsHdrLen + copy(b[dnsHdrLen:], svc)
	local := n
	n += copy(b[n:], wireLocal)
	b[n] = 0
	put16(b[n+1:], dnsTypePTR)
	put16(b[n+3:], dnsClassIN)
	n += 5
	questions := 1
	for i := range found {
		e := &found[i]
		var label []byte
		var to int
		var typ uint16
		switch {
		case e.Port == 0:
			label, to, typ = e.name[:e.nameLen], dnsHdrLen, dnsTypeSRV
		case e.Addr.IsZero() && e.hostLen > 0:
			label, to, typ = e.host[:e.hostLen], local, dnsTypeA
		default:
			continue
		}
		if n+1+len(label)+6 > len(b) {
			break
		}
		b[n] = byte(len(label))
		n += 1 + copy(b[n+1:], label)
		put16(b[n:], 0xc000|uint16(to))
		put16(b[n+2:], typ)
		put16(b[n+4:], dnsClassIN)
		n += 6
		questions++
	}
	put16(b[4:], uint16(questions))
	return n
}

// parseLookup adds to out[found:] the instances of svc of the PTR records of
// the response m, sets the ports and hosts of the SRV records and the
// addresses of the A records. It gives the new number of instances.
func parseLookup(m, svc []byte, out []ServiceInstance, found int) int {
	if len(m) < dnsHdrLen || m[2]&0xf8 != 0x80 || m[3]&0x0f != 0 { // a response to QUERY
		return found
	}
	off := dnsHdrLen
	for q := be16(m[4:]); q > 0; q-- {
		var ok bool
		if off, ok = skipName(m, off); !ok || off+4 > len(m) {
			return found
		}
		off += 4
	}
	records := int(be16(m[6:])) + int(be16(m[8:])) + int(be16(m[10:]))
	svcName, localName := mdnsName{svc, wireLocal}, mdnsName{wireLocal}
	// The PTR records first, then SRV, then A: their order in m can be any.
	for pass := 0; pass < 3; pass++ {
		p := off
		for k := 0; k < records; k++ {
			rr, ok := readRR(m, p)
			if !ok {
				break
			}
			p = rr.next
			if rr.class&^mdnsCacheFlush != dnsClassIN || rr.ttl == 0 {
				continue
			}
			switch {
			case pass == 0 && rr.typ == dnsTypePTR:
				if match, _, _ := matchName(m, rr.name, svcName); !match {
					continue
				}
				label, rest, ok := firstLabel(m, rr.rd)
				if !ok {
					continue
				}
				if match, _, _ := matchName(m, rest, svcName); !match {
					continue
				}
				if findInstance(out[:found], label) < 0 && found < len(out) {
					e := &out[found]
					e.nameLen = uint8(copy(e.name[:], label))
					found++
				}
			case pass == 1 && rr.typ == dnsTypeSRV && rr.rdLen > 6:
				label, rest, ok := firstLabel(m, rr.name)
				if !ok {
					continue
				}
				i := findInstance(out[:found], label)
				if match, _, _ := matchName(m, rest, svcName); !match || i < 0 {
					continue
				}
				host, hostRest, ok := firstLabel(m, rr.rd+6)
				if !ok {
					continue
				}
				if match, _, _ := matchName(m, hostRest, localName); !match {
					continue
				}
				e := &out[i]
				e.Port = be16(m[rr.rd+4:])
				e.hostLen = uint8(copy(e.host[:], host))
			case pass == 2 && rr.typ == dnsTypeA && rr.rdLen == 4:
				label, rest, ok := firstLabel(m, rr.name)
				if !ok {
					continue
				}
				if match, _, _ := matchName(m, rest, localName); !match {
					continue
				}
				for i := range out[:found] {
					e := &out[i]
					if e.hostLen > 0 && foldEq(e.host[:e.hostLen], label) {
						e.Addr = Addr(m[rr.rd : rr.rd+4])
					}
				}
			}
		}
	}
	return found
}

// findInstance gives the index of the instance named label, or -1.
func findInstance(found []ServiceInstance, label []byte) int {
	for i := range found {
		if foldEq(found[i].name[:found[i].nameLen], label) {
			return i
		}
	}
	return -1
}
