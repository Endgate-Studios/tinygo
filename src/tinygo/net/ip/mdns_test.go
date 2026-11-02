package ip

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"runtime/memory/arena"
)

var mdnsMAC = [6]byte{0x01, 0x00, 0x5e, 0x00, 0x00, 0xfb}

func TestMulticast(t *testing.T) {
	s, l, _ := newTestStack(t)
	c, err := s.ListenUDP(5000, 256)
	if err != nil {
		t.Fatal(err)
	}
	group := Addr{224, 0, 0, 200}
	frame := ethernet(multicastMAC(group), peerMAC, etherIPv4, udpPacket(peerIP, group, 5000, 5000, []byte("g")))
	s.Input(frame)
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 8)); ok {
		t.Fatal("a datagram of a group that is not joined came in")
	}
	if err := s.JoinMulticast(group); err != nil {
		t.Fatal(err)
	}
	s.Input(frame)
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 8)); !ok {
		t.Fatal("no datagram of the joined group")
	}
	if err := c.WriteTo([]byte("x"), group, 5000); err != nil {
		t.Fatal(err)
	}
	f := l.next(t)
	if [6]byte(f[0:6]) != multicastMAC(group) || f[14+8] != 255 {
		t.Fatalf("frame to the group: MAC %x TTL %d", f[0:6], f[14+8])
	}
	s.LeaveMulticast(group)
	s.Input(frame)
	if _, _, _, ok, _ := c.TryReadFrom(make([]byte, 8)); ok {
		t.Fatal("a datagram came in after LeaveMulticast")
	}
	if s.JoinMulticast(Addr{192, 168, 1, 1}) != ErrNotMulticast {
		t.Error("JoinMulticast took a unicast address")
	}
}

// mdnsQuery makes a query for name with type typ, as the first question.
func mdnsQuery(id uint16, name string, typ uint16) []byte {
	b := make([]byte, 128)
	n, _ := putQuery(b, name)
	b[2] = 0 // no RD
	put16(b[0:], id)
	put16(b[n-4:], typ)
	return b[:n]
}

// mdnsAddr gives the address of the A record in a response.
func mdnsAddr(t *testing.T, f []byte) (Addr, uint16) {
	t.Helper()
	m := f[14+20+8:]
	off := dnsHdrLen
	var ok bool
	for q := be16(m[4:]); q > 0; q-- {
		if off, ok = skipName(m, off); !ok {
			t.Fatalf("response %x: bad question", m)
		}
		off += 4
	}
	if off, ok = skipName(m, off); !ok || be16(m[6:]) != 1 || off+14 > len(m) {
		t.Fatalf("response %x: no answer", m)
	}
	if be16(m[off:]) != dnsTypeA || be16(m[off+2:])&^mdnsCacheFlush != dnsClassIN || be16(m[off+8:]) != 4 {
		t.Fatalf("response %x: not an A record", m)
	}
	return Addr(m[off+10 : off+14]), be16(m[0:])
}

func TestMDNS(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(peerIP, peerMAC)
	go s.ServeMDNS("tinygo-node")

	// Two announcements, one second apart.
	for i := 0; i < mdnsAnnounces; i++ {
		f := l.next(t)
		if [6]byte(f[0:6]) != mdnsMAC {
			t.Fatalf("announcement %d to %x", i, f[0:6])
		}
		if a, _ := mdnsAddr(t, f); a != myIP {
			t.Fatalf("announcement %d gives %v", i, a)
		}
	}

	query := func(sport uint16, q []byte) {
		s.Input(ethernet(mdnsMAC, peerMAC, etherIPv4, udpPacket(peerIP, MDNSGroup, sport, mdnsPort, q)))
	}
	query(mdnsPort, mdnsQuery(0, "TinyGo-Node.local", dnsTypeA))
	f := l.next(t)
	if a, id := mdnsAddr(t, f); [6]byte(f[0:6]) != mdnsMAC || a != myIP || id != 0 {
		t.Fatalf("multicast answer to %x: %v id %d", f[0:6], a, id)
	}

	// Another name or another type gets no answer.
	query(mdnsPort, mdnsQuery(0, "other.local", dnsTypeA))
	query(mdnsPort, mdnsQuery(0, "tinygo-node.local", 28)) // AAAA
	noSeg(t, l)

	// A legacy resolver on another port gets a unicast answer with its ID.
	query(40000, mdnsQuery(0x1234, "tinygo-node.local", dnsTypeA))
	f = l.next(t)
	a, id := mdnsAddr(t, f)
	if [6]byte(f[0:6]) != peerMAC || a != myIP || id != 0x1234 || be16(f[14+20+2:]) != 40000 {
		t.Fatalf("legacy answer to %x port %d: %v id %#x", f[0:6], be16(f[14+20+2:]), a, id)
	}
	if !bytes.Contains(f, []byte("\x0btinygo-node\x05local")) {
		t.Error("the legacy answer has no question")
	}
}

func TestMDNSCompressedQuestion(t *testing.T) {
	r, err := (&Stack{}).NewMDNS("tinygo-node")
	if err != nil {
		t.Fatal(err)
	}
	// The second question points to the name of the first one.
	q := mdnsQuery(0, "other.local", dnsTypeA)
	put16(q[4:], 2)
	q = append(q, 0x0b)
	q = append(q, "tinygo-node"...)
	q = append(q, 0xc0, dnsHdrLen+6, 0, dnsTypeA, 0x80, dnsClassIN) // .local, QU bit
	// The legacy answer has no pointer into the query: the name is in full
	// in its question, and the answer points there.
	n := r.answer(r.out[:], q, myIP, true)
	if n == 0 {
		t.Fatal("no match for a compressed name")
	}
	m := decodeDNS(t, r.out[:n])
	if len(m.qs) != 1 || m.qs[0] != "tinygo-node.local A" || len(m.an) != 1 || m.an[0].key() != "tinygo-node.local A" || m.an[0].data != "192.168.1.10" {
		t.Errorf("legacy answer %+v", m)
	}
	if c := bytes.Count(r.out[:n], []byte("\x0btinygo-node\x05local\x00")); c != 1 {
		t.Errorf("legacy answer %x has the full name %d times, want 1", r.out[:n], c)
	}
	if r.answer(r.out[:], q[:len(q)-3], myIP, true) != 0 {
		t.Fatal("a short message matched")
	}
	if err := (&Stack{}).ServeMDNS("a.b"); err != ErrBadName {
		t.Errorf("name with a dot: %v", err)
	}
}

// The services of the tests. The AirPlay TXT strings are as long as those
// of a real receiver.
var (
	airplayTXT = []string{
		"acl=0", "btaddr=00:00:00:00:00:00", "deviceid=AA:BB:CC:DD:EE:FF",
		"fex=1c9/St5/FsSsCQ", "features=0x4A7FDFD5,0xBC177FDE", "flags=0x4",
		"gid=5dccfd20-b166-49cc-a593-6abd5f724ddb", "igl=1", "gcgl=1",
		"model=AppleTV3,2", "protovers=1.1", "pi=ba5cb8df-7f14-4249-901a-5e748ce57a93",
		"psi=00000000-0000-0000-0000-AABBCCDDEEFF",
		"pk=99fd4299889422515fbd27949e4e1e21b2af50a454499e3d4be75737bb5f6745",
		"srcvers=366.0", "osvers=8.4.4", "vv=2",
		"comment=" + strings.Repeat("x", 200),
	}
	raopTXT = []string{
		"cn=0,1,2,3", "da=true", "et=0,3,5", "ft=0x4A7FDFD5,0xBC177FDE", "sf=0x4",
		"md=0,1,2", "am=AppleTV3,2", "pk=99fd4299889422515fbd27949e4e1e21b2af50a454499e3d4be75737bb5f6745",
		"tp=UDP", "vn=65537", "vs=366.0", "ov=8.4.4",
	}
)

// newTestMDNS makes a responder on s with four services, its capacity.
func newTestMDNS(tb testing.TB, s *Stack) *MDNS {
	tb.Helper()
	r, err := s.NewMDNS("tinygo-node")
	if err != nil {
		tb.Fatal(err)
	}
	for _, v := range []struct {
		inst, svc string
		port      uint16
		txt       []string
	}{
		{"Living Room", "_airplay._tcp", 7000, airplayTXT},
		{"AABBCCDDEEFF@Living Room", "_raop._tcp", 7000, raopTXT},
		{"Living Room", "_tinsel-surround._tcp", 7100, []string{"v=1"}},
		{"Kitchen", "_airplay._tcp.local.", 7001, nil},
	} {
		if err := r.AddService(v.inst, v.svc, v.port, v.txt...); err != nil {
			tb.Fatalf("AddService %s: %v", v.inst, err)
		}
	}
	return r
}

// testQ is a question of a test message, and testRec a record.
type testQ struct {
	name string
	typ  uint16
	qu   bool
}

type testRec struct {
	name string
	typ  uint16
	ttl  uint32
	data []byte
}

// testName writes a name in full, without compression.
func testName(name string) []byte {
	var b []byte
	for _, l := range strings.Split(name, ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

// testMessage makes a query, or a response if resp, with the records in
// the answer section.
func testMessage(id uint16, resp bool, qs []testQ, an []testRec) []byte {
	b := []byte{byte(id >> 8), byte(id), 0, 0, 0, byte(len(qs)), 0, byte(len(an)), 0, 0, 0, 0}
	if resp {
		b[2] = 0x84
	}
	for _, q := range qs {
		class := uint16(dnsClassIN)
		if q.qu {
			class |= 0x8000
		}
		b = append(b, testName(q.name)...)
		b = append(b, byte(q.typ>>8), byte(q.typ), byte(class>>8), byte(class))
	}
	for _, r := range an {
		b = append(b, testName(r.name)...)
		b = append(b, byte(r.typ>>8), byte(r.typ), 0, dnsClassIN,
			byte(r.ttl>>24), byte(r.ttl>>16), byte(r.ttl>>8), byte(r.ttl),
			byte(len(r.data)>>8), byte(len(r.data)))
		b = append(b, r.data...)
	}
	return b
}

func srvData(port uint16, target string) []byte {
	return append([]byte{0, 0, 0, 0, byte(port >> 8), byte(port)}, testName(target)...)
}

func txtData(txt ...string) []byte {
	var b []byte
	for _, s := range txt {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	return b
}

// decodedRR is a record decoded by decodeDNS.
type decodedRR struct {
	name  string
	typ   uint16
	class uint16
	flush bool
	ttl   uint32
	// The address, the PTR target, "priority weight port target", or the
	// TXT strings joined with "|".
	data string
}

func (r decodedRR) key() string { return r.name + " " + typeName(r.typ) }

type decodedMsg struct {
	id, flags  uint16
	qs         []string // "name TYPE"
	an, ns, ar []decodedRR
}

func typeName(t uint16) string {
	switch t {
	case dnsTypeA:
		return "A"
	case dnsTypePTR:
		return "PTR"
	case dnsTypeSRV:
		return "SRV"
	case dnsTypeTXT:
		return "TXT"
	case dnsTypeANY:
		return "ANY"
	}
	return strconv.Itoa(int(t))
}

// decodeDNS decodes a message apart from the code of the package. A
// compression pointer must point back to the start of a label decoded
// before, the record data must be what its type says, and the message must
// have no bytes after the records.
func decodeDNS(t *testing.T, m []byte) decodedMsg {
	t.Helper()
	u16 := func(off int) uint16 {
		if off+2 > len(m) {
			t.Fatalf("message %x cut at %d", m, off)
		}
		return uint16(m[off])<<8 | uint16(m[off+1])
	}
	starts := map[int]bool{}
	name := func(off int) (string, int) {
		var labels []string
		end := -1
		for hops := 0; ; hops++ {
			if off >= len(m) || hops > 64 {
				t.Fatalf("message %x: bad name at %d", m, off)
			}
			l := int(m[off])
			if l&0xc0 == 0xc0 {
				p := int(u16(off) & 0x3fff)
				if p >= off || !starts[p] {
					t.Fatalf("message %x: the pointer at %d to %d is not to a label before", m, off, p)
				}
				if end < 0 {
					end = off + 2
				}
				off = p
				continue
			}
			if l > 63 || off+1+l > len(m) {
				t.Fatalf("message %x: bad label at %d", m, off)
			}
			starts[off] = true
			if l == 0 {
				if end < 0 {
					end = off + 1
				}
				return strings.Join(labels, "."), end
			}
			labels = append(labels, string(m[off+1:off+1+l]))
			off += 1 + l
		}
	}
	var d decodedMsg
	d.id, d.flags = u16(0), u16(2)
	off := 12
	for i := 0; i < int(u16(4)); i++ {
		var n string
		n, off = name(off)
		d.qs = append(d.qs, n+" "+typeName(u16(off)))
		off += 4
	}
	counts := [3]int{int(u16(6)), int(u16(8)), int(u16(10))}
	for sec, count := range counts {
		for i := 0; i < count; i++ {
			var r decodedRR
			r.name, off = name(off)
			r.typ, r.class = u16(off), u16(off+2)&0x7fff
			r.flush = u16(off+2)&0x8000 != 0
			r.ttl = uint32(u16(off+4))<<16 | uint32(u16(off+6))
			rdLen := int(u16(off + 8))
			rd := off + 10
			off = rd + rdLen
			if off > len(m) {
				t.Fatalf("message %x: record data past the end", m)
			}
			switch r.typ {
			case dnsTypeA:
				if rdLen != 4 {
					t.Fatalf("A record of %d bytes", rdLen)
				}
				r.data = Addr(m[rd:off]).String()
			case dnsTypePTR:
				var end int
				r.data, end = name(rd)
				if end != off {
					t.Fatalf("PTR record of %d bytes, the name has %d", rdLen, end-rd)
				}
			case dnsTypeSRV:
				target, end := name(rd + 6)
				if end != off {
					t.Fatalf("SRV record of %d bytes, the target ends at %d", rdLen, end-rd)
				}
				r.data = strconv.Itoa(int(u16(rd))) + " " + strconv.Itoa(int(u16(rd+2))) + " " + strconv.Itoa(int(u16(rd+4))) + " " + target
			case dnsTypeTXT:
				var txt []string
				for p := rd; p < off; {
					l := int(m[p])
					if p+1+l > off {
						t.Fatalf("TXT string past the record")
					}
					txt = append(txt, string(m[p+1:p+1+l]))
					p += 1 + l
				}
				if len(txt) == 0 {
					t.Fatalf("TXT record with no string")
				}
				r.data = strings.Join(txt, "|")
			}
			switch sec {
			case 0:
				d.an = append(d.an, r)
			case 1:
				d.ns = append(d.ns, r)
			default:
				d.ar = append(d.ar, r)
			}
		}
	}
	if off != len(m) {
		t.Fatalf("message %x: %d bytes after the records", m, len(m)-off)
	}
	return d
}

func keys(rrs []decodedRR) string {
	var k []string
	for _, r := range rrs {
		k = append(k, r.key())
	}
	return strings.Join(k, ", ")
}

// checkRecord checks the TTL, the cache-flush bit and the data of a record
// of the responder of newTestMDNS.
func checkRecord(t *testing.T, r decodedRR, legacy bool) {
	t.Helper()
	data := map[string]string{
		"tinygo-node.local A":                           "192.168.1.10",
		"_airplay._tcp.local PTR":                       "Living Room._airplay._tcp.local|Kitchen._airplay._tcp.local",
		"_raop._tcp.local PTR":                          "AABBCCDDEEFF@Living Room._raop._tcp.local",
		"_tinsel-surround._tcp.local PTR":               "Living Room._tinsel-surround._tcp.local",
		"_services._dns-sd._udp.local PTR":              "_airplay._tcp.local|_raop._tcp.local|_tinsel-surround._tcp.local",
		"Living Room._airplay._tcp.local SRV":           "0 0 7000 tinygo-node.local",
		"Kitchen._airplay._tcp.local SRV":               "0 0 7001 tinygo-node.local",
		"AABBCCDDEEFF@Living Room._raop._tcp.local SRV": "0 0 7000 tinygo-node.local",
		"Living Room._tinsel-surround._tcp.local SRV":   "0 0 7100 tinygo-node.local",
		"Living Room._airplay._tcp.local TXT":           strings.Join(airplayTXT, "|"),
		"Kitchen._airplay._tcp.local TXT":               "",
		"AABBCCDDEEFF@Living Room._raop._tcp.local TXT": strings.Join(raopTXT, "|"),
		"Living Room._tinsel-surround._tcp.local TXT":   "v=1",
	}
	want, ok := data[r.key()]
	if !ok {
		t.Errorf("unexpected record %s", r.key())
		return
	}
	if r.typ == dnsTypePTR {
		if !strings.Contains("|"+want+"|", "|"+r.data+"|") {
			t.Errorf("%s points to %q, want one of %q", r.key(), r.data, want)
		}
	} else if r.data != want {
		t.Errorf("%s has %q, want %q", r.key(), r.data, want)
	}
	unique := r.typ != dnsTypePTR
	ttl := uint32(mdnsOtherTTL)
	if r.typ == dnsTypeA || r.typ == dnsTypeSRV {
		ttl = mdnsTTL
	}
	if legacy {
		ttl, unique = mdnsLegacyTTL, false
	}
	if r.ttl != ttl || r.flush != unique || r.class != dnsClassIN {
		t.Errorf("%s: TTL %d cache-flush %v class %d, want %d %v 1", r.key(), r.ttl, r.flush, r.class, ttl, unique)
	}
}

// checkCompression checks that the names are in full only once.
func checkCompression(t *testing.T, m []byte) {
	t.Helper()
	for _, s := range []string{"\x05local\x00", "\x08_airplay\x04_tcp", "\x0btinygo-node", "\x0bLiving Room"} {
		if c := bytes.Count(m, []byte(s)); c > 1 && !(s == "\x0bLiving Room" && c <= 2) {
			t.Errorf("%q is %d times in the response", s, c)
		}
	}
}

func TestMDNSAnswers(t *testing.T) {
	r := newTestMDNS(t, &Stack{})
	airplay := "_airplay._tcp.local"
	living := "Living Room._airplay._tcp.local"
	kitchen := "Kitchen._airplay._tcp.local"
	ptrData := append([]byte("\x0bLiving Room"), testName(airplay)...)
	for _, tc := range []struct {
		name   string
		qs     []testQ
		known  []testRec
		id     uint16
		legacy bool
		an, ar string
	}{
		{name: "A", qs: []testQ{{"tinygo-node.local", dnsTypeA, false}},
			an: "tinygo-node.local A"},
		{name: "PTR", qs: []testQ{{airplay, dnsTypePTR, false}},
			an: airplay + " PTR, " + airplay + " PTR",
			ar: living + " SRV, " + living + " TXT, " + kitchen + " SRV, " + kitchen + " TXT, tinygo-node.local A"},
		{name: "PTR with the QU bit", qs: []testQ{{"_tinsel-surround._tcp.local", dnsTypePTR, true}},
			an: "_tinsel-surround._tcp.local PTR",
			ar: "Living Room._tinsel-surround._tcp.local SRV, Living Room._tinsel-surround._tcp.local TXT, tinygo-node.local A"},
		{name: "meta-query", qs: []testQ{{"_services._dns-sd._udp.local", dnsTypePTR, false}},
			an: "_services._dns-sd._udp.local PTR, _services._dns-sd._udp.local PTR, _services._dns-sd._udp.local PTR"},
		{name: "SRV", qs: []testQ{{living, dnsTypeSRV, false}},
			an: living + " SRV", ar: "tinygo-node.local A"},
		{name: "TXT", qs: []testQ{{living, dnsTypeTXT, false}},
			an: living + " TXT"},
		{name: "ANY of an instance, other case", qs: []testQ{{"living room._AIRPLAY._tcp.LOCAL", dnsTypeANY, false}},
			an: living + " SRV, " + living + " TXT", ar: "tinygo-node.local A"},
		{name: "two questions", qs: []testQ{{"_raop._tcp.local", dnsTypePTR, false}, {"tinygo-node.local", dnsTypeA, false}},
			an: "tinygo-node.local A, _raop._tcp.local PTR",
			ar: "AABBCCDDEEFF@Living Room._raop._tcp.local SRV, AABBCCDDEEFF@Living Room._raop._tcp.local TXT"},
		{name: "legacy PTR", id: 0x4242, legacy: true, qs: []testQ{{airplay, dnsTypePTR, false}},
			an: airplay + " PTR, " + airplay + " PTR",
			ar: living + " SRV, " + living + " TXT, " + kitchen + " SRV, " + kitchen + " TXT, tinygo-node.local A"},
		{name: "legacy SRV", id: 7, legacy: true, qs: []testQ{{"other.local", dnsTypeA, false}, {living, dnsTypeSRV, false}},
			an: living + " SRV", ar: "tinygo-node.local A"},
		{name: "known PTR", qs: []testQ{{airplay, dnsTypePTR, false}},
			known: []testRec{{airplay, dnsTypePTR, 4500, ptrData}},
			an:    airplay + " PTR",
			ar:    kitchen + " SRV, " + kitchen + " TXT, tinygo-node.local A"},
		{name: "known PTR at less than half its TTL", qs: []testQ{{airplay, dnsTypePTR, false}},
			known: []testRec{{airplay, dnsTypePTR, 2000, ptrData}},
			an:    airplay + " PTR, " + airplay + " PTR",
			ar:    living + " SRV, " + living + " TXT, " + kitchen + " SRV, " + kitchen + " TXT, tinygo-node.local A"},
		{name: "known A", qs: []testQ{{"tinygo-node.local", dnsTypeA, false}},
			known: []testRec{{"tinygo-node.local", dnsTypeA, 120, myIP[:]}}},
		{name: "known A of another address", qs: []testQ{{"tinygo-node.local", dnsTypeA, false}},
			known: []testRec{{"tinygo-node.local", dnsTypeA, 120, peerIP[:]}},
			an:    "tinygo-node.local A"},
		{name: "known SRV and TXT", qs: []testQ{{living, dnsTypeANY, false}},
			known: []testRec{{living, dnsTypeSRV, 120, srvData(7000, "tinygo-node.local")}, {living, dnsTypeTXT, 4500, txtData(airplayTXT...)}}},
		{name: "known TXT of other strings", qs: []testQ{{living, dnsTypeTXT, false}},
			known: []testRec{{living, dnsTypeTXT, 4500, txtData("acl=0")}},
			an:    living + " TXT"},
		{name: "AAAA", qs: []testQ{{"tinygo-node.local", 28, false}}},
		{name: "another service", qs: []testQ{{"_http._tcp.local", dnsTypePTR, false}}},
		{name: "another instance", qs: []testQ{{"Den._airplay._tcp.local", dnsTypeSRV, false}}},
		{name: "PTR of an instance", qs: []testQ{{living, dnsTypePTR, false}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := testMessage(tc.id, false, tc.qs, tc.known)
			n := r.answer(r.out[:], q, myIP, tc.legacy)
			if tc.an == "" {
				if n != 0 {
					t.Fatalf("an answer %x", r.out[:n])
				}
				return
			}
			if n == 0 {
				t.Fatal("no answer")
			}
			out := r.out[:n]
			m := decodeDNS(t, out)
			if m.flags != 0x8400 || len(m.ns) != 0 {
				t.Errorf("flags %#x, %d authority records", m.flags, len(m.ns))
			}
			if got := keys(m.an); got != tc.an {
				t.Errorf("answers %s, want %s", got, tc.an)
			}
			if got := keys(m.ar); got != tc.ar {
				t.Errorf("additional records %s, want %s", got, tc.ar)
			}
			for _, rr := range append(m.an, m.ar...) {
				checkRecord(t, rr, tc.legacy)
			}
			checkCompression(t, out)
			if tc.legacy {
				q := tc.qs[len(tc.qs)-1]
				first := q.name + " " + typeName(q.typ)
				if m.id != tc.id || len(m.qs) != 1 || m.qs[0] != first {
					t.Errorf("legacy answer: ID %#x, questions %q, want %#x and %q", m.id, m.qs, tc.id, first)
				}
			} else if m.id != 0 || len(m.qs) != 0 {
				t.Errorf("multicast answer: ID %#x, questions %q", m.id, m.qs)
			}
		})
	}

	// A response is not a query.
	q := testMessage(0, true, []testQ{{"tinygo-node.local", dnsTypeA, false}}, nil)
	if n := r.answer(r.out[:], q, myIP, false); n != 0 {
		t.Errorf("answer to a response: %x", r.out[:n])
	}
}

// TestMDNSFullAnswer checks that the additional records that do not fit are
// left out, and the message stays valid.
func TestMDNSFullAnswer(t *testing.T) {
	r, _ := (&Stack{}).NewMDNS("tinygo-node")
	big := []string{strings.Repeat("a", 255), strings.Repeat("b", 255), strings.Repeat("c", 255), strings.Repeat("d", 250)}
	for _, inst := range []string{"one", "two", "three"} {
		if err := r.AddService(inst, "_airplay._tcp", 7000, big...); err != nil {
			t.Fatal(err)
		}
	}
	n := r.answer(r.out[:], testMessage(0, false, []testQ{{"_airplay._tcp.local", dnsTypePTR, false}}, nil), myIP, false)
	m := decodeDNS(t, r.out[:n])
	if len(m.an) != 3 || n > mdnsMaxMsg {
		t.Fatalf("%d answers in %d bytes", len(m.an), n)
	}
	want := "one._airplay._tcp.local SRV, one._airplay._tcp.local TXT, two._airplay._tcp.local SRV, three._airplay._tcp.local SRV, tinygo-node.local A"
	if got := keys(m.ar); got != want {
		t.Errorf("additional records %s, want %s", got, want)
	}
}

func TestMDNSServiceErrors(t *testing.T) {
	r := newTestMDNS(t, &Stack{})
	t255 := strings.Repeat("t", 255)
	for _, tc := range []struct {
		name      string
		inst, svc string
		txt       []string
		want      error
	}{
		{"no instance", "", "_airplay._tcp", nil, ErrBadName},
		{"long instance", strings.Repeat("x", 64), "_airplay._tcp", nil, ErrBadName},
		{"no underscore", "a", "airplay._tcp", nil, ErrBadService},
		{"no protocol", "a", "_airplay", nil, ErrBadService},
		{"bad protocol", "a", "_airplay._sctp", nil, ErrBadService},
		{"long type", "a", "_a-very-long-service._tcp", nil, ErrBadService},
		{"three labels", "a", "_x._y._tcp", nil, ErrBadService},
		{"long TXT string", "a", "_airplay._tcp", []string{t255 + "t"}, ErrTXTTooLong},
		{"long TXT", "a", "_airplay._tcp", []string{t255, t255, t255, t255, ""}, ErrTXTTooLong},
		{"registered", "living room", "_airplay._tcp.local.", nil, ErrMDNSExists},
		{"full", "Den", "_airplay._tcp", nil, ErrMDNSFull},
	} {
		if err := r.AddService(tc.inst, tc.svc, 1, tc.txt...); err != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	if err := r.SetTXT("Den", "_airplay._tcp", "a=b"); err != ErrMDNSUnknown {
		t.Errorf("SetTXT of an unknown service: %v", err)
	}
	if err := r.RemoveService("Den", "_airplay._tcp"); err != ErrMDNSUnknown {
		t.Errorf("RemoveService of an unknown service: %v", err)
	}
	if err := r.RemoveService("Kitchen", "_airplay._tcp"); err != nil {
		t.Fatal(err)
	}
	// MDNSMaxTXT exactly.
	if err := r.AddService("Den", "_airplay._tcp", 1, t255, t255, t255, t255); err != nil {
		t.Fatalf("a TXT of %d bytes: %v", MDNSMaxTXT, err)
	}
	if err := r.SetTXT("Living Room", "_airplay._tcp", "a=b", "c"); err != nil {
		t.Fatal(err)
	}
	n := r.answer(r.out[:], testMessage(0, false, []testQ{{"_airplay._tcp.local", dnsTypePTR, false}}, nil), myIP, false)
	m := decodeDNS(t, r.out[:n])
	got := map[string]string{}
	for _, rr := range m.ar {
		got[rr.key()] = rr.data
	}
	if got["Living Room._airplay._tcp.local TXT"] != "a=b|c" || got["Den._airplay._tcp.local TXT"] != strings.Join([]string{t255, t255, t255, t255}, "|") {
		t.Errorf("TXT records after SetTXT and AddService: %v", got)
	}
	if _, ok := got["Kitchen._airplay._tcp.local SRV"]; ok {
		t.Error("a removed service is in the answer")
	}
}

// udpPayload gives the UDP data of a frame and its ports.
func udpPayload(f []byte) (data []byte, sport, dport uint16) {
	u := f[14+20:]
	return u[8:be16(u[4:])], be16(u[0:]), be16(u[2:])
}

func TestMDNSAnnounce(t *testing.T) {
	s, l, _ := newTestStack(t)
	r, err := s.NewMDNS("tinygo-node")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.AddService("Living Room", "_tinsel-surround._tcp", 7100, "v=1"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.Serve() }()

	announcements := func(svc string) {
		t.Helper()
		var at [mdnsAnnounces]time.Time
		for i := range at {
			f := l.next(t)
			at[i] = time.Now()
			data, sport, dport := udpPayload(f)
			if [6]byte(f[0:6]) != mdnsMAC || sport != mdnsPort || dport != mdnsPort {
				t.Fatalf("announcement %d to %x port %d", i, f[0:6], dport)
			}
			m := decodeDNS(t, data)
			inst := "Living Room." + svc
			want := svc + " PTR, " + inst + " SRV, " + inst + " TXT, _services._dns-sd._udp.local PTR, tinygo-node.local A"
			if got := keys(m.an); got != want || len(m.ar) != 0 || m.id != 0 || len(m.qs) != 0 {
				t.Fatalf("announcement %d: %s, want %s", i, got, want)
			}
			for _, rr := range m.an {
				if rr.typ == dnsTypePTR && rr.name == svc && rr.data != inst {
					t.Errorf("PTR to %q", rr.data)
				}
				if rr.ttl == 0 || rr.flush != (rr.typ != dnsTypePTR) {
					t.Errorf("%s: TTL %d cache-flush %v", rr.key(), rr.ttl, rr.flush)
				}
			}
			checkCompression(t, data)
		}
		if d := at[1].Sub(at[0]); d < 900*time.Millisecond || d > 1500*time.Millisecond {
			t.Errorf("announcements %v apart", d)
		}
	}
	announcements("_tinsel-surround._tcp.local")

	// A service added while it serves.
	start := time.Now()
	if err := r.AddService("Living Room", "_airplay._tcp", 7000, airplayTXT...); err != nil {
		t.Fatal(err)
	}
	announcements("_airplay._tcp.local")
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("the announcements of a new service took %v", d)
	}
	noSeg(t, l)

	// A goodbye when a service is removed.
	if err := r.RemoveService("Living Room", "_airplay._tcp"); err != nil {
		t.Fatal(err)
	}
	data, _, _ := udpPayload(l.next(t))
	m := decodeDNS(t, data)
	if got := keys(m.an); got != "_airplay._tcp.local PTR, Living Room._airplay._tcp.local SRV, Living Room._airplay._tcp.local TXT, _services._dns-sd._udp.local PTR" {
		t.Errorf("goodbye: %s", got)
	}
	for _, rr := range m.an {
		if rr.ttl != 0 {
			t.Errorf("goodbye: %s has TTL %d", rr.key(), rr.ttl)
		}
	}

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != ErrClosed {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

// TestMDNSAnswerAllocs checks that the receive and answer path of Serve does
// not allocate.
func TestMDNSAnswerAllocs(t *testing.T) {
	h, err := arena.NewHeap(make([]byte, 16<<10))
	if err != nil {
		t.Fatal(err)
	}
	link := &sinkLink{t: t}
	s := New(link, h)
	s.SetConfig(Config{Addr: myIP, Mask: mask24, Gateway: gwIP})
	s.arp.update(peerIP, peerMAC)
	r := newTestMDNS(t, s)
	c, err := s.ListenUDP(mdnsPort, mdnsRing)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.JoinMulticast(MDNSGroup); err != nil {
		t.Fatal(err)
	}
	ptr := testMessage(0, false, []testQ{{"_airplay._tcp.local", dnsTypePTR, true}}, nil)
	meta := testMessage(0, false, []testQ{{"_services._dns-sd._udp.local", dnsTypePTR, false}}, nil)
	known := testMessage(0, false, []testQ{{"tinygo-node.local", dnsTypeA, false}}, []testRec{{"tinygo-node.local", dnsTypeA, 120, peerIP[:]}})
	legacy := testMessage(9, false, []testQ{{"Living Room._airplay._tcp.local", dnsTypeANY, false}}, nil)
	frames := [][]byte{
		ethernet(mdnsMAC, peerMAC, etherIPv4, udpPacket(peerIP, MDNSGroup, mdnsPort, mdnsPort, ptr)),
		ethernet(mdnsMAC, peerMAC, etherIPv4, udpPacket(peerIP, MDNSGroup, mdnsPort, mdnsPort, meta)),
		ethernet(mdnsMAC, peerMAC, etherIPv4, udpPacket(peerIP, MDNSGroup, mdnsPort, mdnsPort, known)),
		ethernet(mdnsMAC, peerMAC, etherIPv4, udpPacket(peerIP, MDNSGroup, 40000, mdnsPort, legacy)),
	}
	answered := 0
	allocs := testing.AllocsPerRun(100, func() {
		for _, f := range frames {
			link.last = link.last[:0]
			s.Input(f)
			n, from, port, ok, err := c.TryReadFrom(r.in[:])
			if !ok || err != nil {
				t.Fatal("no query received")
			}
			if err := r.handle(c, r.in[:n], from, port); err != nil {
				t.Fatal(err)
			}
			if len(link.last) > 0 {
				answered++
			}
		}
	})
	if allocs != 0 {
		t.Errorf("%v allocations for each run of 4 queries, want 0", allocs)
	}
	if answered != 4*101 {
		t.Errorf("%d answers, want %d", answered, 4*101)
	}
}

func TestLookupMDNS(t *testing.T) {
	gw, gwLink := newPipeStack(t, [6]byte{0x02, 0, 0, 0, 0, 0xaa})
	gw.SetConfig(Config{Addr: Addr{192, 168, 4, 1}, Mask: mask24})
	sp, spLink := newPipeStack(t, [6]byte{0x02, 0, 0, 0, 0, 0x01})
	sp.SetConfig(Config{Addr: Addr{192, 168, 4, 2}, Mask: mask24})
	done := make(chan struct{})
	defer close(done)
	go pipe(gwLink, sp, done)
	go pipe(spLink, gw, done)

	r, err := gw.NewMDNS("gateway")
	if err != nil {
		t.Fatal(err)
	}
	r.AddService("Living Room", "_airplay._tcp", 7000, airplayTXT...)
	r.AddService("Living Room", "_tinsel-surround._tcp", 7100, "v=1")
	r.AddService("Kitchen", "_tinsel-surround._tcp", 7200)
	go r.Serve()
	defer r.Close()

	living, kitchen := "Living Room gateway 192.168.4.1:7100", "Kitchen gateway 192.168.4.1:7200"
	for _, tc := range []struct {
		name    string
		svc     string
		max     int
		timeout time.Duration
		want    string // instances "name host addr:port", the first one of the answer if max is 1
		err     error
		quick   bool
	}{
		{"first one", "_tinsel-surround._tcp", 1, 3 * time.Second, living, nil, true},
		{"all", "_tinsel-surround._tcp.local", 4, 1500 * time.Millisecond, living + ", " + kitchen, nil, false},
		{"none", "_nothing._tcp", 2, time.Second, "", ErrNotFound, false},
		{"bad type", "nothing", 2, time.Second, "", ErrBadService, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := make([]ServiceInstance, tc.max)
			start := time.Now()
			n, err := sp.LookupMDNS(tc.svc, out, tc.timeout)
			took := time.Since(start)
			if err != tc.err {
				t.Fatalf("error %v, want %v", err, tc.err)
			}
			if tc.quick && took > 500*time.Millisecond {
				t.Errorf("took %v", took)
			}
			var got []string
			for _, e := range out[:n] {
				got = append(got, e.Name()+" "+e.Host()+" "+e.Addr.String()+":"+strconv.Itoa(int(e.Port)))
			}
			if s := strings.Join(got, ", "); s != tc.want {
				t.Errorf("found %q, want %q", s, tc.want)
			}
		})
	}
}

// TestLookupMDNSFollowUp answers a lookup with hand-made responses that
// lack the SRV and A records, so it asks for them.
func TestLookupMDNSFollowUp(t *testing.T) {
	s, l, _ := newTestStack(t)
	type result struct {
		n   int
		err error
	}
	out := make([]ServiceInstance, 1)
	res := make(chan result, 1)
	go func() {
		n, err := s.LookupMDNS("_tinsel-surround._tcp", out, 5*time.Second)
		res <- result{n, err}
	}()
	svc := "_tinsel-surround._tcp.local"
	query := func(want string) uint16 {
		t.Helper()
		f := l.next(t)
		data, sport, dport := udpPayload(f)
		if [6]byte(f[0:6]) != mdnsMAC || dport != mdnsPort || sport == mdnsPort {
			t.Fatalf("query to %x from port %d to %d", f[0:6], sport, dport)
		}
		m := decodeDNS(t, data)
		if got := strings.Join(m.qs, ", "); m.flags != 0 || got != want {
			t.Fatalf("query %#x %q, want %q", m.flags, got, want)
		}
		return sport
	}
	respond := func(port uint16, m []byte) {
		s.Input(ethernet(myMAC, peerMAC, etherIPv4, udpPacket(peerIP, myIP, mdnsPort, port, m)))
	}

	port := query(svc + " PTR")
	// A PTR record of another service, then one whose target points to
	// the name of the record.
	respond(port, testMessage(0, true, nil, []testRec{{"_http._tcp.local", dnsTypePTR, 4500, testName("Web._http._tcp.local")}}))
	respond(port, testMessage(0, true, nil, []testRec{{svc, dnsTypePTR, 4500, []byte{3, 'D', 'e', 'n', 0xc0, dnsHdrLen}}}))
	query(svc + " PTR, Den." + svc + " SRV")
	respond(port, testMessage(0, true, nil, []testRec{{"Den." + svc, dnsTypeSRV, 120, srvData(7100, "speaker-gw.local")}}))
	query(svc + " PTR, speaker-gw.local A")
	respond(port, testMessage(0, true, nil, []testRec{
		{"other.local", dnsTypeA, 120, []byte{10, 0, 0, 9}},
		{"Speaker-GW.local", dnsTypeA, 120, []byte{192, 168, 1, 50}},
	}))
	select {
	case r := <-res:
		if r.err != nil || r.n != 1 {
			t.Fatalf("LookupMDNS: %d, %v", r.n, r.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("LookupMDNS did not return")
	}
	if e := out[0]; e.Name() != "Den" || e.Host() != "speaker-gw" || e.Addr != (Addr{192, 168, 1, 50}) || e.Port != 7100 {
		t.Errorf("found %q on %q at %v:%d", e.Name(), e.Host(), e.Addr, e.Port)
	}
}
