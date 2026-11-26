// Package x509 parses the parts of an X.509 certificate (RFC 5280) that a
// TLS client needs: the signed part, the signature, the names, the validity,
// a P-256, P-384 or RSA public key, the DNS names and the CA flag. The fields
// refer to the DER bytes, so Parse does not allocate.
package x509

import (
	"time"

	"runtime/memory"
	"tinygo/crypto/p256"
	"tinygo/crypto/p384"
	"tinygo/crypto/rsa"
	"tinygo/crypto/sha256"
	"tinygo/crypto/sha512"
)

// Memory is where the signature checks get their working memory.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrFormat   Error = "x509: bad certificate"
	ErrCritical Error = "x509: unknown critical extension"
	ErrKey      Error = "x509: the key is not on P-256"
)

// Signature algorithms.
const (
	SigUnknown = iota
	SigECDSASHA256
	SigECDSASHA384
	SigRSASHA256
	SigRSASHA384
	SigRSASHA512
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
	P384      bool          // the key is an EC key on P-384
	Key384    []byte        // x and y of the P-384 key, 96 bytes of the DER
	RSA       bool          // the key is an RSA key
	RSAKey    rsa.PublicKey // N refers to the DER bytes
	dnsNames  []byte        // the content of the SAN extension, or nil
	IsCA      bool
	// MaxPathLen is the pathLenConstraint of a CA, or -1 without one.
	MaxPathLen int
	// KeyUsage has the bits of the keyUsage extension, digitalSignature
	// first, when HasKeyUsage is set (RFC 5280 section 4.2.1.3).
	KeyUsage    uint16
	HasKeyUsage bool
	// ServerAuth tells that the extKeyUsage extension allows TLS servers,
	// when HasExtKeyUsage is set (RFC 5280 section 4.2.1.12).
	ServerAuth     bool
	HasExtKeyUsage bool
}

// Bits of KeyUsage.
const (
	UsageDigitalSignature = 0x8000
	UsageCertSign         = 0x0400
)

// CanSign reports whether c is a CA that may sign certificates.
func (c *Certificate) CanSign() bool {
	return c.IsCA && (!c.HasKeyUsage || c.KeyUsage&UsageCertSign != 0)
}

// ForServer reports whether the key of c may sign for a TLS 1.3 server: the
// extended key usage allows serverAuth and the key usage digitalSignature.
func (c *Certificate) ForServer() bool {
	return (!c.HasExtKeyUsage || c.ServerAuth) && (!c.HasKeyUsage || c.KeyUsage&UsageDigitalSignature != 0)
}

// Object identifiers, the DER content of the OID.
var (
	oidECDSASHA256 = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x04, 0x03, 0x02}
	oidECDSASHA384 = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x04, 0x03, 0x03}
	oidRSASHA256   = []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0b}
	oidRSASHA384   = []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0c}
	oidRSASHA512   = []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x0d}
	oidRSA         = []byte{0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x01, 0x01}
	oidECPublicKey = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01}
	oidP256        = []byte{0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07}
	oidP384        = []byte{0x2b, 0x81, 0x04, 0x00, 0x22}
	oidSAN         = []byte{0x55, 0x1d, 0x11}
	oidBasic       = []byte{0x55, 0x1d, 0x13}
	oidKeyUsage    = []byte{0x55, 0x1d, 0x0f}
	oidExtKeyUsage = []byte{0x55, 0x1d, 0x25}
	oidAnyUsage    = []byte{0x55, 0x1d, 0x25, 0x00}
	oidServerAuth  = []byte{0x2b, 0x06, 0x01, 0x05, 0x05, 0x07, 0x03, 0x01}
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
	*c = Certificate{Raw: der, MaxPathLen: -1}
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
	case equal(oid, oidRSASHA256):
		c.SigAlg = SigRSASHA256
	case equal(oid, oidRSASHA384):
		c.SigAlg = SigRSASHA384
	case equal(oid, oidRSASHA512):
		c.SigAlg = SigRSASHA512
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

// parseKey reads an EC key on P-256 or P-384 in uncompressed form, or an RSA
// key.
func parseKey(spki []byte, c *Certificate) {
	alg, _, rest, ok := expect(spki, tagSequence)
	if !ok {
		return
	}
	oid, _, params, ok := expect(alg, tagOID)
	if ok && equal(oid, oidRSA) {
		parseRSAKey(rest, c)
		return
	}
	if !ok || !equal(oid, oidECPublicKey) {
		return
	}
	curve, _, _, ok := expect(params, tagOID)
	if !ok {
		return
	}
	key, _, _, ok := expect(rest, tagBitString)
	if ok && equal(curve, oidP384) && len(key) == 2+2*p384.Size && key[0] == 0 && key[1] == 4 {
		c.Key384 = key[2:]
		c.P384 = p384.OnCurve((*[p384.Size]byte)(key[2:]), (*[p384.Size]byte)(key[2+p384.Size:]))
		return
	}
	if !ok || !equal(curve, oidP256) || len(key) != 66 || key[0] != 0 || key[1] != 4 {
		return
	}
	copy(c.KeyX[:], key[2:34])
	copy(c.KeyY[:], key[34:66])
	c.P256 = p256.OnCurve(&c.KeyX, &c.KeyY)
}

// parseRSAKey reads the BIT STRING with SEQUENCE { n INTEGER, e INTEGER }.
func parseRSAKey(b []byte, c *Certificate) {
	key, _, _, ok := expect(b, tagBitString)
	if !ok || len(key) < 1 || key[0] != 0 {
		return
	}
	seq, _, _, ok := expect(key[1:], tagSequence)
	if !ok {
		return
	}
	n, _, rest, ok := expect(seq, tagInteger)
	if !ok {
		return
	}
	e, _, _, ok := expect(rest, tagInteger)
	if !ok || len(e) == 0 || len(e) > 4 || e[0]&0x80 != 0 {
		return
	}
	for len(n) > 0 && n[0] == 0 {
		n = n[1:]
	}
	if len(n) < 128 || len(n)*8 > rsa.MaxBits {
		return // below 1024 bits, or too large
	}
	var ev uint32
	for _, x := range e {
		ev = ev<<8 | uint32(x)
	}
	c.RSAKey = rsa.PublicKey{N: n, E: ev}
	c.RSA = true
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
		critical := false
		if len(r) > 0 && r[0] == tagBoolean {
			var v []byte
			if v, _, r, ok = expect(r, tagBoolean); !ok || len(v) != 1 {
				return ErrFormat
			}
			critical = v[0] != 0
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
			if ca, _, rest, ok := expect(bc, tagBoolean); ok {
				c.IsCA = len(ca) == 1 && ca[0] != 0
				bc = rest
			}
			if n, _, _, ok := expect(bc, tagInteger); ok {
				if len(n) == 0 || len(n) > 2 || n[0]&0x80 != 0 {
					return ErrFormat
				}
				c.MaxPathLen = int(n[0])
				if len(n) == 2 {
					c.MaxPathLen = c.MaxPathLen<<8 | int(n[1])
				}
			}
		case equal(oid, oidKeyUsage):
			bits, _, _, ok := expect(value, tagBitString)
			if !ok || len(bits) < 2 || len(bits) > 3 {
				return ErrFormat
			}
			c.KeyUsage = uint16(bits[1]) << 8
			if len(bits) == 3 {
				c.KeyUsage |= uint16(bits[2])
			}
			c.HasKeyUsage = true
		case equal(oid, oidExtKeyUsage):
			list, _, _, ok := expect(value, tagSequence)
			if !ok {
				return ErrFormat
			}
			for len(list) > 0 {
				u, _, rest, ok := expect(list, tagOID)
				if !ok {
					return ErrFormat
				}
				list = rest
				if equal(u, oidServerAuth) || equal(u, oidAnyUsage) {
					c.ServerAuth = true
				}
			}
			c.HasExtKeyUsage = true
		default:
			if critical {
				return ErrCritical
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
// host. A wildcard covers one label at the left, as RFC 6125 6.4.3. When
// host is an IPv4 address, only an iPAddress of the SAN extension matches.
func (c *Certificate) VerifyHostname(host string) bool {
	ip, isIP := ParseIPv4(host)
	names := c.dnsNames
	for len(names) > 0 {
		tag, name, _, rest, ok := next(names)
		if !ok {
			return false
		}
		names = rest
		switch {
		case isIP:
			// An address matches only an iPAddress (RFC 6125 section 6.2.1).
			if tag == 0x87 && len(name) == 4 && [4]byte(name) == ip {
				return true
			}
		case tag == 0x82 && matchHost(name, host): // dNSName [2]
			return true
		}
	}
	return false
}

// ParseIPv4 reads a dotted IPv4 address such as 192.168.1.10.
func ParseIPv4(s string) ([4]byte, bool) {
	var ip [4]byte
	k, v, digits := 0, 0, 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '.' {
			if digits == 0 || k == 4 {
				return ip, false
			}
			ip[k] = byte(v)
			k, v, digits = k+1, 0, 0
			continue
		}
		d := s[i] - '0'
		if d > 9 || (digits > 0 && v == 0) { // no leading zeros
			return ip, false
		}
		v, digits = v*10+int(d), digits+1
		if v > 255 {
			return ip, false
		}
	}
	return ip, k == 4
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

// CheckSignatureFrom reports whether parent, a CA, signed c with ECDSA on
// P-256 or P-384 with SHA-256 or SHA-384, or with RSA PKCS #1 v1.5 and SHA-256,
// SHA-384 or SHA-512.
func (c *Certificate) CheckSignatureFrom(mem Memory, parent *Certificate) bool {
	if !parent.CanSign() || !equal(c.Issuer, parent.Subject) {
		return false
	}
	switch {
	case parent.P256 && c.SigAlg == SigECDSASHA256:
		return VerifyECDSA(mem, c.TBS, c.Sig, &parent.KeyX, &parent.KeyY)
	case parent.P256 && c.SigAlg == SigECDSASHA384:
		var h [sha512.Size384]byte
		var r, s [32]byte
		sha512.Sum384(&h, c.TBS)
		return parseSig(c.Sig, r[:], s[:]) && p256.VerifyECDSA(mem, h[:], &r, &s, &parent.KeyX, &parent.KeyY)
	case parent.P384:
		return VerifyECDSA384(c.SigAlg, c.TBS, c.Sig, parent.Key384)
	case parent.RSA:
		return VerifyRSA(mem, c.SigAlg, c.TBS, c.Sig, &parent.RSAKey)
	}
	return false
}

// VerifyRSA checks a PKCS #1 v1.5 signature over msg, with the hash of alg:
// SigRSASHA256, SigRSASHA384 or SigRSASHA512.
func VerifyRSA(mem Memory, alg int, msg, sig []byte, k *rsa.PublicKey) bool {
	var h [sha512.Size]byte
	var hash []byte
	switch alg {
	case SigRSASHA256:
		var d sha256.Digest
		d.Reset(sha256.Init, 0)
		d.Write(msg)
		d.Sum((*[sha256.Size]byte)(h[:]))
		hash = h[:sha256.Size]
	case SigRSASHA384, SigRSASHA512:
		var d sha512.Digest
		init, n := sha512.Init384, sha512.Size384
		if alg == SigRSASHA512 {
			init, n = sha512.Init512, sha512.Size
		}
		d.Reset(init, 0)
		d.Write(msg)
		d.Sum(&h)
		hash = h[:n]
	default:
		return false
	}
	return rsa.VerifyPKCS1v15Digest(mem, k, hash, sig)
}

// VerifyECDSA checks a DER ECDSA-Sig-Value over the SHA-256 of msg.
func VerifyECDSA(mem Memory, msg, sig []byte, x, y *[32]byte) bool {
	var r, s [32]byte
	if !parseSig(sig, r[:], s[:]) {
		return false
	}
	var d sha256.Digest
	d.Reset(sha256.Init, 0)
	d.Write(msg)
	var h [sha256.Size]byte
	d.Sum(&h)
	return p256.VerifyECDSA(mem, h[:], &r, &s, x, y)
}

// VerifyECDSA384 checks a DER ECDSA-Sig-Value over msg with key, the 96 bytes
// of x and y of a P-384 key. alg is SigECDSASHA384 or SigECDSASHA256.
func VerifyECDSA384(alg int, msg, sig, key []byte) bool {
	var r, s [p384.Size]byte
	if len(key) != 2*p384.Size || !parseSig(sig, r[:], s[:]) {
		return false
	}
	var h [sha512.Size384]byte
	hash := h[:]
	switch alg {
	case SigECDSASHA384:
		sha512.Sum384(&h, msg)
	case SigECDSASHA256:
		var d sha256.Digest
		d.Reset(sha256.Init, 0)
		d.Write(msg)
		d.Sum((*[sha256.Size]byte)(h[:]))
		hash = h[:sha256.Size]
	default:
		return false
	}
	return p384.VerifyECDSA(hash, &r, &s, (*[p384.Size]byte)(key), (*[p384.Size]byte)(key[p384.Size:]))
}

// parseSig reads SEQUENCE { r INTEGER, s INTEGER } into r and s, of the same
// length.
func parseSig(sig []byte, r, s []byte) bool {
	seq, _, _, ok := expect(sig, tagSequence)
	if !ok {
		return false
	}
	ri, _, rest, ok := expect(seq, tagInteger)
	if !ok {
		return false
	}
	si, _, _, ok := expect(rest, tagInteger)
	return ok && intTo(ri, r) && intTo(si, s)
}

// intTo copies a positive DER INTEGER of at most len(out) bytes, without its
// sign byte.
func intTo(v []byte, out []byte) bool {
	if len(v) > 0 && v[0] == 0 {
		v = v[1:]
	}
	if len(v) == 0 || len(v) > len(out) {
		return false
	}
	clear(out)
	copy(out[len(out)-len(v):], v)
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
