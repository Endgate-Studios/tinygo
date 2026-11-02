package ip

import (
	"sync"
	"time"
)

// A Multicast DNS responder (RFC 6762) for the A record of one host name and
// the DNS-SD services of the host (RFC 6763). It does not probe for name
// conflicts (section 8.1), does not delay the answers of shared records
// (section 6) and does not wait for the rest of a truncated query (section
// 7.2). It has only IPv4 records: an AAAA query gets no answer.

const (
	mdnsPort       = 5353
	mdnsTTL        = 120  // host records, A and SRV, RFC 6762 section 10
	mdnsOtherTTL   = 4500 // other records, PTR and TXT
	mdnsLegacyTTL  = 10   // unicast answers to other ports, section 6.7
	mdnsCacheFlush = 0x8000
	mdnsAnnounces  = 2 // section 8.3
	mdnsMaxMsg     = mtu - ipLen - udpLen
	mdnsRing       = 4096
	mdnsMaxSvcName = 24 // "_" and 15 characters, then "_tcp", as labels
	dnsTypePTR     = 12
	dnsTypeTXT     = 16
	dnsTypeSRV     = 33
	dnsTypeANY     = 255
	dnsClassANY    = 255
)

const (
	// MDNSMaxServices is the number of services an MDNS responder holds.
	MDNSMaxServices = 4
	// MDNSMaxTXT is the size of the TXT record of a service: each string
	// takes its length and one byte.
	MDNSMaxTXT = 1024
)

const (
	ErrMDNSFull    Error = "ip: too many mDNS services"
	ErrMDNSExists  Error = "ip: mDNS service already registered"
	ErrMDNSUnknown Error = "ip: mDNS service not registered"
	ErrBadService  Error = "ip: bad service type"
	ErrTXTTooLong  Error = "ip: TXT record too long"
)

// MDNSGroup is the IPv4 group of Multicast DNS.
var MDNSGroup = Addr{224, 0, 0, 251}

var (
	wireLocal = []byte("\x05local")
	wireMeta  = []byte("\x09_services\x07_dns-sd\x04_udp") // RFC 6763 section 9
)

// MDNS is a Multicast DNS responder. It answers for the A record of its host
// name and for the PTR, SRV and TXT records of its services, which it
// announces when they are added. Its methods are safe for concurrent use:
// add services before or while Serve runs. Answering a query does not
// allocate.
type MDNS struct {
	s     *Stack
	host  [64]byte // the host label, after its length
	hostN int

	mu      sync.Mutex
	conn    *UDPConn // while Serve runs
	svcs    [MDNSMaxServices]mdnsService
	pending uint8 // a bit for each service to announce
	left    int   // announcements left
	next    time.Time
	w       mdnsWriter
	out     [mdnsMaxMsg]byte

	in [mdnsMaxMsg]byte // only for Serve
}

type mdnsService struct {
	used  bool
	inst  [64]byte // the instance label, after its length
	instN int
	svc   [mdnsMaxSvcName]byte // two labels, as "\x08_airplay\x04_tcp"
	svcN  int
	port  uint16
	txt   [MDNSMaxTXT]byte
	txtN  int
}

// NewMDNS makes a responder for host.local. host is one DNS label, for
// example "tinygo-node". Add services, then call Serve.
func (s *Stack) NewMDNS(host string) (*MDNS, error) {
	if len(host) == 0 || len(host) > 63 {
		return nil, ErrBadName
	}
	for i := 0; i < len(host); i++ {
		if host[i] == '.' {
			return nil, ErrBadName
		}
	}
	r := &MDNS{s: s}
	r.host[0] = byte(len(host))
	r.hostN = 1 + copy(r.host[1:], host)
	return r, nil
}

// ServeMDNS answers the queries for the A record of name.local with the
// address of the stack, and announces it at the start. name is one DNS label,
// for example "tinygo-node". It returns only on an error. Use NewMDNS to
// advertise services too.
func (s *Stack) ServeMDNS(name string) error {
	r, err := s.NewMDNS(name)
	if err != nil {
		return err
	}
	return r.Serve()
}

// AddService advertises the instance of service on port, with the strings of
// txt in its TXT record, and announces it twice, one second apart. instance
// is one label of up to 63 bytes, which can hold spaces and dots, for example
// "Living Room". service is the type, for example "_airplay._tcp". The TXT
// strings are usually "key=value", of up to 255 bytes each and MDNSMaxTXT in
// all.
func (r *MDNS) AddService(instance, service string, port uint16, txt ...string) error {
	if len(instance) == 0 || len(instance) > 63 {
		return ErrBadName
	}
	var svc [mdnsMaxSvcName]byte
	svcN, err := putServiceType(svc[:], service)
	if err != nil {
		return err
	}
	if err := checkTXT(txt); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.find(instance, svc[:svcN]) >= 0 {
		return ErrMDNSExists
	}
	i := 0
	for i < len(r.svcs) && r.svcs[i].used {
		i++
	}
	if i == len(r.svcs) {
		return ErrMDNSFull
	}
	v := &r.svcs[i]
	v.used = true
	v.inst[0] = byte(len(instance))
	v.instN = 1 + copy(v.inst[1:], instance)
	v.svcN = copy(v.svc[:], svc[:svcN])
	v.port = port
	v.txtN = putTXT(v.txt[:], txt)
	r.announceSoon(i)
	return nil
}

// SetTXT replaces the TXT strings of a service and announces the change.
func (r *MDNS) SetTXT(instance, service string, txt ...string) error {
	var svc [mdnsMaxSvcName]byte
	svcN, err := putServiceType(svc[:], service)
	if err != nil {
		return err
	}
	if err := checkTXT(txt); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.find(instance, svc[:svcN])
	if i < 0 {
		return ErrMDNSUnknown
	}
	r.svcs[i].txtN = putTXT(r.svcs[i].txt[:], txt)
	r.announceSoon(i)
	return nil
}

// RemoveService stops the advertisement of a service. While Serve runs, it
// sends the records of the service with a TTL of 0 (RFC 6762 section 10.1).
func (r *MDNS) RemoveService(instance, service string) error {
	var svc [mdnsMaxSvcName]byte
	svcN, err := putServiceType(svc[:], service)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.find(instance, svc[:svcN])
	if i < 0 {
		return ErrMDNSUnknown
	}
	if me := r.s.Config().Addr; r.conn != nil && !me.IsZero() {
		n := r.unsolicited(me, i, true)
		err = r.conn.WriteTo(r.out[:n], MDNSGroup, mdnsPort)
	}
	r.svcs[i].used = false
	r.pending &^= 1 << i
	return err
}

// Serve answers the queries and sends the announcements, first of the host
// name or of the services added before. It returns on an error, or with
// ErrClosed after Close.
func (r *MDNS) Serve() error {
	c, err := r.s.ListenUDP(mdnsPort, mdnsRing)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := r.s.JoinMulticast(MDNSGroup); err != nil {
		return err
	}
	defer r.s.LeaveMulticast(MDNSGroup)
	r.mu.Lock()
	r.conn = c
	r.left, r.next = mdnsAnnounces, time.Now()
	r.mu.Unlock()
	defer r.detach()

	for {
		r.mu.Lock()
		deadline, err := r.announce(c)
		r.mu.Unlock()
		if err != nil {
			return err
		}
		n, from, port, ok, err := c.TryReadFrom(r.in[:])
		if err != nil {
			return err
		}
		if !ok {
			// AddService wakes it up for its announcements.
			wait(c.ready, deadline)
			continue
		}
		if err := r.handle(c, r.in[:n], from, port); err != nil {
			return err
		}
	}
}

// Close stops Serve.
func (r *MDNS) Close() error {
	r.mu.Lock()
	c := r.conn
	r.mu.Unlock()
	if c == nil {
		return ErrClosed
	}
	return c.Close()
}

func (r *MDNS) detach() {
	r.mu.Lock()
	r.conn = nil
	r.mu.Unlock()
}

// announceSoon schedules the announcements of service i and wakes up Serve.
// The caller holds r.mu.
func (r *MDNS) announceSoon(i int) {
	r.pending |= 1 << i
	r.left, r.next = mdnsAnnounces, time.Now()
	if r.conn != nil {
		select {
		case r.conn.ready <- struct{}{}:
		default:
		}
	}
}

// announce sends the announcements that are due and gives when to call it
// again, zero for never. A message has the records of one service and the
// host, so a large TXT record fits. The caller holds r.mu.
func (r *MDNS) announce(c *UDPConn) (time.Time, error) {
	if r.left == 0 {
		return time.Time{}, nil
	}
	now := time.Now()
	if now.Before(r.next) {
		return r.next, nil
	}
	me := r.s.Config().Addr
	if me.IsZero() {
		return now.Add(time.Second), nil
	}
	if r.pending == 0 {
		n := r.unsolicited(me, -1, false)
		if err := c.WriteTo(r.out[:n], MDNSGroup, mdnsPort); err != nil {
			return time.Time{}, err
		}
	}
	for i := range r.svcs {
		if r.pending&(1<<i) == 0 {
			continue
		}
		n := r.unsolicited(me, i, false)
		if err := c.WriteTo(r.out[:n], MDNSGroup, mdnsPort); err != nil {
			return time.Time{}, err
		}
	}
	r.left--
	if r.left == 0 {
		r.pending = 0
		return time.Time{}, nil
	}
	r.next = now.Add(time.Second)
	return r.next, nil
}

// handle answers the query q from the address and port.
func (r *MDNS) handle(c *UDPConn, q []byte, from Addr, port uint16) error {
	me := r.s.Config().Addr
	if me.IsZero() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	legacy := port != mdnsPort
	n := r.answer(r.out[:], q, me, legacy)
	if n == 0 {
		return nil
	}
	to, toPort := MDNSGroup, uint16(mdnsPort)
	if legacy {
		// A legacy resolver: a unicast answer with its ID and question.
		to, toPort = from, port
	}
	if err := c.WriteTo(r.out[:n], to, toPort); err != nil && err != ErrNoARP {
		return err
	}
	return nil
}

// find gives the index of a service, or -1. The caller holds r.mu.
func (r *MDNS) find(instance string, svc []byte) int {
	for i := range r.svcs {
		v := &r.svcs[i]
		if v.used && v.instN == 1+len(instance) && equalFold(v.inst[1:v.instN], instance) && foldEq(v.svc[:v.svcN], svc) {
			return i
		}
	}
	return -1
}

// sameType reports whether the services i and j have the same type.
func (r *MDNS) sameType(i, j int) bool {
	return foldEq(r.svcs[i].svc[:r.svcs[i].svcN], r.svcs[j].svc[:r.svcs[j].svcN])
}

// firstOfType reports whether service i is the first one of its type: it
// gives the PTR record of the type for the meta-query.
func (r *MDNS) firstOfType(i int) bool {
	for j := 0; j < i; j++ {
		if r.svcs[j].used && r.sameType(i, j) {
			return false
		}
	}
	return true
}

// lastOfType reports whether no other service has the type of service i.
func (r *MDNS) lastOfType(i int) bool {
	for j := range r.svcs {
		if j != i && r.svcs[j].used && r.sameType(i, j) {
			return false
		}
	}
	return true
}

// mdnsName is a name as up to three runs of wire labels, without the final
// zero.
type mdnsName [3][]byte

func (r *MDNS) hostName() mdnsName { return mdnsName{r.host[:r.hostN], wireLocal} }

func (v *mdnsService) svcName() mdnsName { return mdnsName{v.svc[:v.svcN], wireLocal} }

func (v *mdnsService) instName() mdnsName {
	return mdnsName{v.inst[:v.instN], v.svc[:v.svcN], wireLocal}
}

const (
	nameNone = iota
	nameHost
	nameMeta
	nameSvc  // the type of the services of the mask
	nameInst // the instance of the service of the mask
)

// classify gives which of the names of the responder is at off, and the
// offset after the name.
func (r *MDNS) classify(m []byte, off int) (kind int, mask uint8, next int, ok bool) {
	match, next, ok := matchName(m, off, r.hostName())
	if !ok {
		return nameNone, 0, 0, false
	}
	if match {
		return nameHost, 0, next, true
	}
	if match, _, _ := matchName(m, off, mdnsName{wireMeta, wireLocal}); match {
		return nameMeta, 0, next, true
	}
	for i := range r.svcs {
		v := &r.svcs[i]
		if !v.used {
			continue
		}
		if match, _, _ := matchName(m, off, v.svcName()); match {
			mask |= 1 << i
		}
	}
	if mask != 0 {
		return nameSvc, mask, next, true
	}
	for i := range r.svcs {
		v := &r.svcs[i]
		if !v.used {
			continue
		}
		if match, _, _ := matchName(m, off, v.instName()); match {
			return nameInst, 1 << i, next, true
		}
	}
	return nameNone, 0, next, true
}

// mdnsSet is a set of records: a bit for each service.
type mdnsSet struct {
	a                   bool
	meta, ptr, srv, txt uint8
}

func (s mdnsSet) empty() bool { return !s.a && s.meta|s.ptr|s.srv|s.txt == 0 }

// answer writes into b the response to the query q and gives its length, 0
// for no response. A legacy response is a unicast one to a resolver on
// another port than 5353: it repeats the ID and the first question it
// answers, and has short TTLs and no cache-flush bit (RFC 6762 section 6.7).
// The caller holds r.mu.
func (r *MDNS) answer(b, q []byte, me Addr, legacy bool) int {
	if len(q) < dnsHdrLen || q[2]&0xf8 != 0 { // a response, or not QUERY
		return 0
	}
	var want mdnsSet
	echoKind, echoMask, echoAt := nameNone, uint8(0), 0
	off := dnsHdrLen
	for n := be16(q[4:]); n > 0; n-- {
		kind, mask, next, ok := r.classify(q, off)
		if !ok || next+4 > len(q) {
			return 0
		}
		typ, class := be16(q[next:]), be16(q[next+2:])&^mdnsCacheFlush // the QU bit
		off = next + 4
		if class != dnsClassIN && class != dnsClassANY {
			continue
		}
		before := want
		all := typ == dnsTypeANY
		switch kind {
		case nameHost:
			want.a = want.a || typ == dnsTypeA || all
		case nameMeta:
			if typ == dnsTypePTR || all {
				for i := range r.svcs {
					if r.svcs[i].used && r.firstOfType(i) {
						want.meta |= 1 << i
					}
				}
			}
		case nameSvc:
			if typ == dnsTypePTR || all {
				want.ptr |= mask
			}
		case nameInst:
			if typ == dnsTypeSRV || all {
				want.srv |= mask
			}
			if typ == dnsTypeTXT || all {
				want.txt |= mask
			}
		}
		if echoKind == nameNone && want != before {
			echoKind, echoMask, echoAt = kind, mask, next
		}
	}
	r.knownAnswers(q, off, me, &want)
	if want.empty() {
		return 0
	}

	w := &r.w
	w.reset(b, legacy)
	if legacy {
		put16(b[0:], be16(q[0:]))
		w.question(r, echoKind, echoMask, q[echoAt:echoAt+4])
	}
	// The answers, then the additional records of RFC 6763 section 12.
	var sent mdnsSet
	sent.a = want.a && w.recA(r, me, mdnsTTL)
	for i := range r.svcs {
		bit := uint8(1) << i
		if want.meta&bit != 0 && w.recMeta(r, i, mdnsOtherTTL) {
			sent.meta |= bit
		}
		if want.ptr&bit != 0 && w.recPTR(r, i, mdnsOtherTTL) {
			sent.ptr |= bit
		}
		if want.srv&bit != 0 && w.recSRV(r, i, mdnsTTL) {
			sent.srv |= bit
		}
		if want.txt&bit != 0 && w.recTXT(r, i, mdnsOtherTTL) {
			sent.txt |= bit
		}
	}
	if w.an == 0 {
		return 0
	}
	w.extra = true
	for i := range r.svcs {
		bit := uint8(1) << i
		if sent.ptr&bit != 0 && sent.srv&bit == 0 && w.recSRV(r, i, mdnsTTL) {
			sent.srv |= bit
		}
		if sent.ptr&bit != 0 && sent.txt&bit == 0 {
			w.recTXT(r, i, mdnsOtherTTL)
		}
	}
	if sent.srv != 0 && !sent.a {
		w.recA(r, me, mdnsTTL)
	}
	put16(b[6:], uint16(w.an))
	put16(b[10:], uint16(w.ar))
	return w.n
}

// knownAnswers takes out of want the records of the answer section of the
// query q, at off, that the querier has with at least half of their TTL
// (RFC 6762 section 7.1).
func (r *MDNS) knownAnswers(q []byte, off int, me Addr, want *mdnsSet) {
	for n := be16(q[6:]); n > 0; n-- {
		rr, ok := readRR(q, off)
		if !ok {
			return
		}
		off = rr.next
		if rr.class&^mdnsCacheFlush != dnsClassIN {
			continue
		}
		kind, mask, _, ok := r.classify(q, rr.name)
		if !ok {
			return
		}
		switch {
		case kind == nameHost && rr.typ == dnsTypeA && rr.ttl >= mdnsTTL/2:
			if rr.rdLen == 4 && Addr(q[rr.rd:rr.rd+4]) == me {
				want.a = false
			}
		case kind == nameMeta && rr.typ == dnsTypePTR && rr.ttl >= mdnsOtherTTL/2:
			for i := range r.svcs {
				if want.meta&(1<<i) != 0 {
					if match, _, _ := matchName(q, rr.rd, r.svcs[i].svcName()); match {
						want.meta &^= 1 << i
					}
				}
			}
		case kind == nameSvc && rr.typ == dnsTypePTR && rr.ttl >= mdnsOtherTTL/2:
			for i := range r.svcs {
				if mask&want.ptr&(1<<i) != 0 {
					if match, _, _ := matchName(q, rr.rd, r.svcs[i].instName()); match {
						want.ptr &^= 1 << i
					}
				}
			}
		case kind == nameInst && rr.typ == dnsTypeSRV && rr.ttl >= mdnsTTL/2 && rr.rdLen > 6:
			v := &r.svcs[bitIndex(mask)]
			if match, _, _ := matchName(q, rr.rd+6, r.hostName()); match && be16(q[rr.rd+4:]) == v.port {
				want.srv &^= mask
			}
		case kind == nameInst && rr.typ == dnsTypeTXT && rr.ttl >= mdnsOtherTTL/2:
			v := &r.svcs[bitIndex(mask)]
			if bytesEq(q[rr.rd:rr.next], v.txt[:v.txtN]) {
				want.txt &^= mask
			}
		}
	}
}

// bitIndex gives the index of the lowest bit of mask.
func bitIndex(mask uint8) int {
	i := 0
	for mask != 0 && mask&1 == 0 {
		mask >>= 1
		i++
	}
	return i
}

// unsolicited writes an announcement into r.out: the A record of the host,
// after the records of service i unless it is -1. A goodbye has the records
// of the service with a TTL of 0, and not the A record. The caller holds
// r.mu.
func (r *MDNS) unsolicited(me Addr, i int, goodbye bool) int {
	w := &r.w
	w.reset(r.out[:], false)
	if i >= 0 {
		other, host := uint32(mdnsOtherTTL), uint32(mdnsTTL)
		if goodbye {
			other, host = 0, 0
		}
		w.recPTR(r, i, other)
		w.recSRV(r, i, host)
		w.recTXT(r, i, other)
		if !goodbye || r.lastOfType(i) {
			w.recMeta(r, i, other)
		}
	}
	if !goodbye {
		w.recA(r, me, mdnsTTL)
	}
	put16(r.out[6:], uint16(w.an))
	return w.n
}

// mdnsWriter writes a response with name compression. It keeps the offset
// of each name it wrote, 0 for none, to point to it the next time. A record
// that does not fit is left out.
type mdnsWriter struct {
	b      []byte
	n      int
	over   bool // the record being written does not fit
	legacy bool
	extra  bool // in the additional section
	an, ar int

	local, host, meta int
	svc, inst         [MDNSMaxServices]int
}

// reset starts a response in b with ID 0.
func (w *mdnsWriter) reset(b []byte, legacy bool) {
	*w = mdnsWriter{b: b, n: dnsHdrLen, legacy: legacy}
	clear(b[:dnsHdrLen])
	b[2] = 0x84 // QR and AA
}

func (w *mdnsWriter) put(p []byte) {
	if w.over || w.n+len(p) > len(w.b) {
		w.over = true
		return
	}
	w.n += copy(w.b[w.n:], p)
}

func (w *mdnsWriter) put8(v byte) {
	if w.over || w.n+1 > len(w.b) {
		w.over = true
		return
	}
	w.b[w.n] = v
	w.n++
}

func (w *mdnsWriter) put16(v uint16) {
	w.put8(byte(v >> 8))
	w.put8(byte(v))
}

// pointer writes a pointer to the name at *off if it is not 0, and reports
// whether it did. Else it sets *off to where the name will be.
func (w *mdnsWriter) pointer(off *int) bool {
	if *off != 0 {
		w.put16(0xc000 | uint16(*off))
		return true
	}
	*off = w.n
	return false
}

func (w *mdnsWriter) localName() {
	if !w.pointer(&w.local) {
		w.put(wireLocal)
		w.put8(0)
	}
}

func (w *mdnsWriter) hostName(r *MDNS) {
	if !w.pointer(&w.host) {
		w.put(r.host[:r.hostN])
		w.localName()
	}
}

func (w *mdnsWriter) metaName() {
	if !w.pointer(&w.meta) {
		w.put(wireMeta)
		w.localName()
	}
}

func (w *mdnsWriter) svcName(r *MDNS, i int) {
	for j := range w.svc {
		if j != i && w.svc[j] != 0 && r.sameType(i, j) {
			w.put16(0xc000 | uint16(w.svc[j]))
			return
		}
	}
	if !w.pointer(&w.svc[i]) {
		w.put(r.svcs[i].svc[:r.svcs[i].svcN])
		w.localName()
	}
}

func (w *mdnsWriter) instName(r *MDNS, i int) {
	if !w.pointer(&w.inst[i]) {
		w.put(r.svcs[i].inst[:r.svcs[i].instN])
		w.svcName(r, i)
	}
}

// question writes the question of a legacy response: the name of kind, and
// the type and class of the query.
func (w *mdnsWriter) question(r *MDNS, kind int, mask uint8, typeClass []byte) {
	switch kind {
	case nameHost:
		w.hostName(r)
	case nameMeta:
		w.metaName()
	case nameSvc:
		w.svcName(r, bitIndex(mask))
	case nameInst:
		w.instName(r, bitIndex(mask))
	}
	w.put(typeClass)
	put16(w.b[4:], 1)
}

// head writes the type, class, TTL and a zero length of a record after its
// name, and gives the offset of the data. Unique records have the
// cache-flush bit.
func (w *mdnsWriter) head(typ uint16, unique bool, ttl uint32) int {
	class := uint16(dnsClassIN)
	if w.legacy {
		ttl = min(ttl, mdnsLegacyTTL)
	} else if unique {
		class |= mdnsCacheFlush
	}
	w.put16(typ)
	w.put16(class)
	w.put16(uint16(ttl >> 16))
	w.put16(uint16(ttl))
	w.put16(0)
	return w.n
}

// end writes the data length and counts the record, or takes the writer back
// to snap if the record does not fit.
func (w *mdnsWriter) end(snap mdnsWriter, rd int) bool {
	if w.over {
		*w = snap
		return false
	}
	put16(w.b[rd-2:], uint16(w.n-rd))
	if w.extra {
		w.ar++
	} else {
		w.an++
	}
	return true
}

func (w *mdnsWriter) recA(r *MDNS, me Addr, ttl uint32) bool {
	snap := *w
	w.hostName(r)
	rd := w.head(dnsTypeA, true, ttl)
	w.put(me[:])
	return w.end(snap, rd)
}

func (w *mdnsWriter) recPTR(r *MDNS, i int, ttl uint32) bool {
	snap := *w
	w.svcName(r, i)
	rd := w.head(dnsTypePTR, false, ttl)
	w.instName(r, i)
	return w.end(snap, rd)
}

func (w *mdnsWriter) recMeta(r *MDNS, i int, ttl uint32) bool {
	snap := *w
	w.metaName()
	rd := w.head(dnsTypePTR, false, ttl)
	w.svcName(r, i)
	return w.end(snap, rd)
}

func (w *mdnsWriter) recSRV(r *MDNS, i int, ttl uint32) bool {
	snap := *w
	w.instName(r, i)
	rd := w.head(dnsTypeSRV, true, ttl)
	w.put16(0) // priority
	w.put16(0) // weight
	w.put16(r.svcs[i].port)
	w.hostName(r)
	return w.end(snap, rd)
}

func (w *mdnsWriter) recTXT(r *MDNS, i int, ttl uint32) bool {
	snap := *w
	w.instName(r, i)
	rd := w.head(dnsTypeTXT, true, ttl)
	w.put(r.svcs[i].txt[:r.svcs[i].txtN])
	return w.end(snap, rd)
}

// putServiceType writes a service type such as "_airplay._tcp", with or
// without ".local", as two labels, and gives their length.
func putServiceType(b []byte, s string) (int, error) {
	if len(s) > 0 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if len(s) > 6 && foldString(s[len(s)-6:], ".local") {
		s = s[:len(s)-6]
	}
	dot := 0
	for dot < len(s) && s[dot] != '.' {
		dot++
	}
	name, proto := s[:dot], ""
	if dot < len(s) {
		proto = s[dot+1:]
	}
	if len(name) < 2 || len(name) > 16 || name[0] != '_' || !(foldString(proto, "_tcp") || foldString(proto, "_udp")) {
		return 0, ErrBadService
	}
	b[0] = byte(len(name))
	n := 1 + copy(b[1:], name)
	b[n] = 4
	n += 1 + copy(b[n+1:], proto)
	return n, nil
}

func checkTXT(txt []string) error {
	n := 0
	for _, s := range txt {
		if len(s) > 255 {
			return ErrTXTTooLong
		}
		n += 1 + len(s)
	}
	if n > MDNSMaxTXT {
		return ErrTXTTooLong
	}
	return nil
}

// putTXT writes the TXT strings, or one empty string for none (RFC 6763
// section 6.1).
func putTXT(b []byte, txt []string) int {
	if len(txt) == 0 {
		b[0] = 0
		return 1
	}
	n := 0
	for _, s := range txt {
		b[n] = byte(len(s))
		n += 1 + copy(b[n+1:], s)
	}
	return n
}

// dnsRR is a resource record of a message, by offsets.
type dnsRR struct {
	name, rd, rdLen, next int
	typ, class            uint16
	ttl                   uint32
}

// readRR reads the record at off.
func readRR(m []byte, off int) (dnsRR, bool) {
	end, ok := skipName(m, off)
	if !ok || end+10 > len(m) {
		return dnsRR{}, false
	}
	rr := dnsRR{name: off, typ: be16(m[end:]), class: be16(m[end+2:]), ttl: be32(m[end+4:])}
	rr.rdLen = int(be16(m[end+8:]))
	rr.rd = end + 10
	rr.next = rr.rd + rr.rdLen
	return rr, rr.next <= len(m)
}

// matchName reports whether the name at off is name, without case, and gives
// the offset after the name. It follows compression pointers.
func matchName(m []byte, off int, name mdnsName) (match bool, next int, ok bool) {
	pi, po := 0, 0 // the next label of name
	next = -1
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
			for pi < len(name) && po >= len(name[pi]) {
				pi, po = pi+1, 0
			}
			return match && pi == len(name), next, true
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
		if match {
			for pi < len(name) && po >= len(name[pi]) {
				pi, po = pi+1, 0
			}
			if pi == len(name) {
				match = false
			} else {
				p := name[pi]
				tl := int(p[po])
				match = foldEq(m[off+1:off+1+l], p[po+1:po+1+tl])
				po += 1 + tl
			}
		}
		off += 1 + l
	}
	return false, 0, false
}

// firstLabel gives the first label of the name at off and the offset of the
// rest of the name.
func firstLabel(m []byte, off int) (label []byte, rest int, ok bool) {
	for jumps := 0; jumps < 16; jumps++ {
		if off >= len(m) {
			return nil, 0, false
		}
		l := int(m[off])
		if l&0xc0 == 0xc0 {
			if off+2 > len(m) {
				return nil, 0, false
			}
			off = int(be16(m[off:]) & 0x3fff)
			continue
		}
		if l == 0 || l&0xc0 != 0 || off+1+l > len(m) {
			return nil, 0, false
		}
		return m[off+1 : off+1+l], off + 1 + l, true
	}
	return nil, 0, false
}

func equalFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if lower(b[i]) != lower(s[i]) {
			return false
		}
	}
	return true
}

func foldString(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func foldEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func bytesEq(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		c += 'a' - 'A'
	}
	return c
}
