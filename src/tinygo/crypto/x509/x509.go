// Package x509 parses the parts of an X.509 certificate (RFC 5280) that a
// TLS client needs: the signed part, the signature, the names, the validity,
// a P-256 public key, the DNS names and the CA flag. The fields refer to the
// DER bytes, so Parse does not allocate.
package x509

import (
	"time"

	"tinygo/crypto/p256"
	"tinygo/crypto/sha256"
)

// Error is a string error type, so error values are constants in flash.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrFormat Error = "x509: bad certificate"
	ErrKey    Error = "x509: the key is not on P-256"
)

// Signature algorithms.
const (
	SigUnknown = iota
	SigECDSASHA256
	SigECDSASHA384
)

// Certificate is a parsed certificate. Its slices refer to the DER bytes.
type Certificate struct {
	Raw       []byte
	TBS       []byte // the signed part, with its tag and length
	SigAlg    int
	Sig       []byte // the BIT STRING content of the signature, without the unused bits byte
	Issuer    []byte // the DER Name, with its tag and length
	Subject   []byte
	NotBefore time.Time
	NotAfter  time.Time
	SPKI      []byte // SubjectPublicKeyInfo, with its tag and length
	P256      bool   // the key is an EC key on P-256
	KeyX      [32]byte
	KeyY      [32]byte
	dnsNames  []byte // the content of the SAN extension, or nil
	IsCA      bool
}

// Object identifiers, the DER content of the OID.
var (
	oidECDSASHA256 = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x04, 0x03, 0x02}
	oidECDSASHA384 = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x04, 0x03, 0x03}
	oidECPublicKey = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01}
	oidP256        = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07}
	oidSAN         = []byte{0x55, 0x1d, 0x11}
	oidBasic       = []byte{0x55, 0x1d, 0x13}
)

// DER tags.
const (
	tagBoolean     = 0x01
	tagInteger     = 0x02
	tagBitString   = 0x03
	tagOctetString = 0x04
	tagOID         = 0x06
	tagUTCTime     = 0x17
	tagGenTime     = 0x18
	tagSequence    = 0x30
	tagExplicit0   = 0xa0
	tagExplicit3   = 0xa3
)

// next reads one TLV from b. It gives the tag, the content, the whole TLV and
// the rest.
func next(b []byte) (tag byte, content, whole, rest []byte, ok bool) {
	if len(b) < 2 {
		return 0, nil, nil, nil, false
	}
	tag = b[0]
	n, h := int(b[1]), 2
	if n&0x80 != 0 {
		k := n & 0x7f
		if k == 0 || k > 3 || len(b) < 2+k {
			return 0, nil, nil, nil, false
		}
		n = 0
		for i := 0; i < k; i++ {
			n = n<<8 | int(b[2+i])
		}
		h += k
	}
	if n > len(b)-h {
		return 0, nil, nil, nil, false
	}
	return tag, b[h : h+n], b[:h+n], b[h+n:], true
}

// expect reads a TLV with the tag want.
func expect(b []byte, want byte) (content, whole, rest []byte, ok bool) {
	tag, c, w, r, ok := next(b)
	if !ok || tag != want {
		return nil, nil, nil, false
	}
	return c, w, r, true
}

// Parse parses der into c.
func Parse(der []byte, c *Certificate) error {
	*c = Certificate{Raw: der}
	cert, _, _, ok := expect(der, tagSequence)
	if !ok {
		return ErrFormat
	}
	tbs, tbsWhole, rest, ok := expect(cert, tagSequence)
	if !ok {
		return ErrFormat
	}
	c.TBS = tbsWhole
	alg, _, rest, ok := expect(rest, tagSequence)
	if !ok {
		return ErrFormat
	}
	oid, _, _, ok := expect(alg, tagOID)
	if !ok {
		return ErrFormat
	}
	switch {
	case equal(oid, oidECDSASHA256):
		c.SigAlg = SigECDSASHA256
	case equal(oid, oidECDSASHA384):
		c.SigAlg = SigECDSASHA384
	}
	sig, _, _, ok := expect(rest, tagBitString)
	if !ok || len(sig) < 1 || sig[0] != 0 {
		return ErrFormat
	}
	c.Sig = sig[1:]
	return parseTBS(tbs, c)
}

func parseTBS(b []byte, c *Certificate) error {
	var ok bool
	if len(b) > 0 && b[0] == tagExplicit0 { // version
		if _, _, b, ok = expect(b, tagExplicit0); !ok {
			return ErrFormat
		}
	}
	if _, _, b, ok = expect(b, tagInteger); !ok { // serial number
		return ErrFormat
	}
	if _, _, b, ok = expect(b, tagSequence); !ok { // signature algorithm
		return ErrFormat
	}
	if _, c.Issuer, b, ok = expect(b, tagSequence); !ok {
		return ErrFormat
	}
	validity, _, b, ok := expect(b, tagSequence)
	if !ok {
		return ErrFormat
	}
	if c.NotBefore, validity, ok = parseTime(validity); !ok {
		return ErrFormat
	}
	if c.NotAfter, _, ok = parseTime(validity); !ok {
		return ErrFormat
	}
	if _, c.Subject, b, ok = expect(b, tagSequence); !ok {
		return ErrFormat
	}
	var spki []byte
	if spki, c.SPKI, b, ok = expect(b, tagSequence); !ok {
		return ErrFormat
	}
	parseKey(spki, c)
	// Skip the unique IDs, then read the extensions.
	for len(b) > 0 {
		tag, content, _, rest, ok := next(b)
		if !ok {
			return ErrFormat
		}
		b = rest
		if tag == tagExplicit3 {
			if err := parseExtensions(content, c); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseKey reads an EC key on P-256 in uncompressed form.
func parseKey(spki []byte, c *Certificate) {
	alg, _, rest, ok := expect(spki, tagSequence)
	if !ok {
		return
	}
	oid, _, params, ok := expect(alg, tagOID)
	if !ok || !equal(oid, oidECPublicKey) {
		return
	}
	curve, _, _, ok := expect(params, tagOID)
	if !ok || !equal(curve, oidP256) {
		return
	}
	key, _, _, ok := expect(rest, tagBitString)
	if !ok || len(key) != 66 || key[0] != 0 || key[1] != 4 {
		return
	}
	copy(c.KeyX[:], key[2:34])
	copy(c.KeyY[:], key[34:66])
	c.P256 = p256.OnCurve(&c.KeyX, &c.KeyY)
}

func parseExtensions(b []byte, c *Certificate) error {
	list, _, _, ok := expect(b, tagSequence)
	if !ok {
		return ErrFormat
	}
	for len(list) > 0 {
		ext, _, rest, ok := expect(list, tagSequence)
		if !ok {
			return ErrFormat
		}
		list = rest
		oid, _, r, ok := expect(ext, tagOID)
		if !ok {
			return ErrFormat
		}
		if len(r) > 0 && r[0] == tagBoolean { // critical
			if _, _, r, ok = expect(r, tagBoolean); !ok {
				return ErrFormat
			}
		}
		value, _, _, ok := expect(r, tagOctetString)
		if !ok {
			return ErrFormat
		}
		switch {
		case equal(oid, oidSAN):
			names, _, _, ok := expect(value, tagSequence)
			if !ok {
				return ErrFormat
			}
			c.dnsNames = names
		case equal(oid, oidBasic):
			bc, _, _, ok := expect(value, tagSequence)
			if !ok {
				return ErrFormat
			}
			if ca, _, _, ok := expect(bc, tagBoolean); ok && len(ca) == 1 && ca[0] != 0 {
				c.IsCA = true
			}
		}
	}
	return nil
}

// parseTime reads a UTCTime or a GeneralizedTime in the Z form.
func parseTime(b []byte) (time.Time, []byte, bool) {
	tag, s, _, rest, ok := next(b)
	if !ok {
		return time.Time{}, nil, false
	}
	var year int
	switch {
	case tag == tagUTCTime && len(s) == 13:
		year = digits(s[0:2])
		if year < 50 { // RFC 5280 4.1.2.5.1
			year += 2000
		} else {
			year += 1900
		}
		s = s[2:]
	case tag == tagGenTime && len(s) == 15:
		year = digits(s[0:4])
		s = s[4:]
	default:
		return time.Time{}, nil, false
	}
	if year < 0 || s[10] != 'Z' {
		return time.Time{}, nil, false
	}
	mo, d, h, mi, sec := digits(s[0:2]), digits(s[2:4]), digits(s[4:6]), digits(s[6:8]), digits(s[8:10])
	if mo < 1 || mo > 12 || d < 1 || d > 31 || h < 0 || h > 23 || mi < 0 || mi > 59 || sec < 0 || sec > 59 {
		return time.Time{}, nil, false
	}
	return time.Date(year, time.Month(mo), d, h, mi, sec, 0, time.UTC), rest, true
}

// digits gives the value of decimal digits, or -1.
func digits(b []byte) int {
	v := 0
	for _, ch := range b {
		if ch < '0' || ch > '9' {
			return -1
		}
		v = v*10 + int(ch-'0')
	}
	return v
}

// VerifyHostname reports whether a DNS name of the SAN extension matches
// host. A wildcard covers one label at the left, as RFC 6125 6.4.3.
func (c *Certificate) VerifyHostname(host string) bool {
	names := c.dnsNames
	for len(names) > 0 {
		tag, name, _, rest, ok := next(names)
		if !ok {
			return false
		}
		names = rest
		if tag == 0x82 && matchHost(name, host) { // dNSName [2]
			return true
		}
	}
	return false
}

func matchHost(pattern []byte, host string) bool {
	if len(pattern) > 2 && pattern[0] == '*' && pattern[1] == '.' {
		dot := -1
		for i := 0; i < len(host); i++ {
			if host[i] == '.' {
				dot = i
				break
			}
		}
		if dot <= 0 {
			return false
		}
		return equalFold(pattern[1:], host[dot:])
	}
	return equalFold(pattern, host)
}

// CheckSignatureFrom reports whether parent signed c with ECDSA P-256 and
// SHA-256, and parent is a CA.
func (c *Certificate) CheckSignatureFrom(mem p256.Memory, parent *Certificate) bool {
	if !parent.P256 || !parent.IsCA || c.SigAlg != SigECDSASHA256 || !equal(c.Issuer, parent.Subject) {
		return false
	}
	return VerifyECDSA(mem, c.TBS, c.Sig, &parent.KeyX, &parent.KeyY)
}

// VerifyECDSA checks a DER ECDSA-Sig-Value over the SHA-256 of msg.
func VerifyECDSA(mem p256.Memory, msg, sig []byte, x, y *[32]byte) bool {
	var r, s [32]byte
	if !parseSig(sig, &r, &s) {
		return false
	}
	var d sha256.Digest
	d.Reset(sha256.Init, 0)
	d.Write(msg)
	var h [sha256.Size]byte
	d.Sum(&h)
	return p256.VerifyECDSA(mem, h[:], &r, &s, x, y)
}

// parseSig reads SEQUENCE { r INTEGER, s INTEGER } into 32-byte values.
func parseSig(sig []byte, r, s *[32]byte) bool {
	seq, _, _, ok := expect(sig, tagSequence)
	if !ok {
		return false
	}
	ri, _, rest, ok := expect(seq, tagInteger)
	if !ok {
		return false
	}
	si, _, _, ok := expect(rest, tagInteger)
	return ok && intTo32(ri, r) && intTo32(si, s)
}

// intTo32 copies a positive DER INTEGER of at most 32 bytes, without its
// sign byte.
func intTo32(v []byte, out *[32]byte) bool {
	if len(v) > 0 && v[0] == 0 {
		v = v[1:]
	}
	if len(v) == 0 || len(v) > 32 {
		return false
	}
	*out = [32]byte{}
	copy(out[32-len(v):], v)
	return true
}

func equal(a, b []byte) bool {
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

func equalFold(a []byte, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
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
