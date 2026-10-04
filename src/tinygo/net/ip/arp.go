package ip

import "time"

// ARP of RFC 826 for IPv4 over Ethernet.

const (
	arpRequest = 1
	arpReply   = 2
	arpLen     = 28
	arpEntries = 8
	arpTries   = 3
	arpWait    = 500 * time.Millisecond
	arpPoll    = 10 * time.Millisecond
	arpMaxAge  = 5 * time.Minute
)

type arpEntry struct {
	addr Addr
	mac  [6]byte
	seen time.Time
}

// arpCache is a fixed table. A new entry replaces the oldest one.
type arpCache struct {
	e [arpEntries]arpEntry
}

func (c *arpCache) lookup(a Addr) ([6]byte, bool) {
	for i := range c.e {
		if e := &c.e[i]; e.addr == a && !a.IsZero() && time.Since(e.seen) < arpMaxAge {
			return e.mac, true
		}
	}
	return [6]byte{}, false
}

func (c *arpCache) update(a Addr, mac [6]byte) {
	oldest := 0
	for i := range c.e {
		e := &c.e[i]
		if e.addr == a {
			oldest = i
			break
		}
		if e.addr.IsZero() || e.seen.Before(c.e[oldest].seen) {
			oldest = i
		}
	}
	c.e[oldest] = arpEntry{addr: a, mac: mac, seen: time.Now()}
}

// inputARP answers requests for our address and learns the sender.
func (s *Stack) inputARP(p []byte) {
	if len(p) < arpLen || be16(p[0:]) != 1 || be16(p[2:]) != etherIPv4 || p[4] != 6 || p[5] != 4 {
		s.stats.Dropped++
		return
	}
	op := be16(p[6:])
	sha, spa, tpa := [6]byte(p[8:14]), Addr(p[14:18]), Addr(p[24:28])
	me := s.cfg.Addr
	if me.IsZero() {
		return
	}
	// RFC 826: update an entry that exists, and add one when the request is
	// for us.
	if _, ok := s.arp.lookup(spa); ok || tpa == me {
		s.arp.update(spa, sha)
	}
	if op == arpRequest && tpa == me {
		s.sendARP(arpReply, sha, spa)
		s.stats.ARPReplies++
	}
}

// sendARP sends a request for tpa, or a reply to tha at tpa. The caller holds
// s.mu.
func (s *Stack) sendARP(op uint16, tha [6]byte, tpa Addr) {
	p := s.tx[ethLen : ethLen+arpLen]
	put16(p[0:], 1) // Ethernet
	put16(p[2:], etherIPv4)
	p[4], p[5] = 6, 4
	put16(p[6:], op)
	copy(p[8:14], s.mac[:])
	copy(p[14:18], s.cfg.Addr[:])
	copy(p[18:24], tha[:])
	copy(p[24:28], tpa[:])
	dst := tha
	if op == arpRequest {
		dst = broadcastMAC
	}
	s.sendEthernet(dst, etherARP, arpLen)
}
