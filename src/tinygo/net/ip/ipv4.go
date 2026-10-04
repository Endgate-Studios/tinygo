package ip

// IPv4 of RFC 791 without fragments and options, and ICMP echo of RFC 792.

const (
	icmpEchoReply   = 0
	icmpEchoRequest = 8
	flagMF          = 0x2000
	fragOffsetMask  = 0x1fff
)

func (s *Stack) inputIPv4(srcMAC [6]byte, p []byte) {
	if len(p) < ipLen || p[0]>>4 != 4 {
		s.stats.Dropped++
		return
	}
	hl := int(p[0]&0x0f) * 4
	total := int(be16(p[2:]))
	if hl < ipLen || total < hl || total > len(p) || fold(checksum(0, p[:hl])) != 0 {
		s.stats.Dropped++
		return
	}
	if be16(p[6:])&(flagMF|fragOffsetMask) != 0 {
		s.stats.Dropped++ // fragments are not supported
		return
	}
	src, dst := Addr(p[12:16]), Addr(p[16:20])
	me := s.cfg.Addr
	if dst != me && !s.isBroadcast(dst) && !(me.IsZero() && p[9] == protoUDP) {
		return // the DHCP client accepts UDP before it has an address
	}
	payload := p[hl:total]
	switch p[9] {
	case protoICMP:
		s.inputICMP(srcMAC, src, dst, payload)
	case protoUDP:
		s.inputUDP(src, dst, payload)
	case protoTCP:
		s.inputTCP(srcMAC, src, dst, payload)
	}
}

// inputICMP answers echo requests. The reply goes to the MAC of the request,
// because Input cannot wait for ARP.
func (s *Stack) inputICMP(srcMAC [6]byte, src, dst Addr, p []byte) {
	if len(p) < 8 || fold(checksum(0, p)) != 0 {
		s.stats.Dropped++
		return
	}
	if p[0] != icmpEchoRequest || dst != s.cfg.Addr || len(p) > mtu-ipLen {
		return
	}
	out := s.tx[ethLen+ipLen : ethLen+ipLen+len(p)]
	copy(out, p)
	out[0] = icmpEchoReply
	out[2], out[3] = 0, 0
	put16(out[2:], fold(checksum(0, out)))
	s.putIPv4Header(s.cfg.Addr, src, protoICMP, len(p))
	if s.sendEthernet(srcMAC, etherIPv4, ipLen+len(p)) == nil {
		s.stats.EchoReplies++
	}
}

func (s *Stack) putIPv4Header(src, dst Addr, proto byte, n int) {
	h := s.tx[ethLen : ethLen+ipLen]
	h[0], h[1] = 0x45, 0
	put16(h[2:], uint16(ipLen+n))
	s.ipID++
	put16(h[4:], s.ipID)
	put16(h[6:], 0)
	h[8], h[9] = defaultTTL, proto
	h[10], h[11] = 0, 0
	copy(h[12:16], src[:])
	copy(h[16:20], dst[:])
	put16(h[10:], fold(checksum(0, h)))
}
