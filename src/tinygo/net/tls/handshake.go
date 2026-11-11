package tls

import (
	"crypto/subtle"
	"io"

	"runtime/memory"
	"tinygo/crypto/hkdf"
	"tinygo/crypto/p256"
	"tinygo/crypto/rsa"
	"tinygo/crypto/sha256"
	"tinygo/crypto/x25519"
	"tinygo/crypto/x509"
)

// Handshake message types (section 4).
const (
	hsClientHello         = 1
	hsServerHello         = 2
	hsNewSessionTicket    = 4
	hsEncryptedExtensions = 8
	hsCertificate         = 11
	hsCertificateVerify   = 15
	hsFinished            = 20
	hsMessageHash         = 254
	hsKeyUpdate           = 24

	// TLS 1.2 only (RFC 5246 section 7.4).
	hsHelloRequest       = 0
	hsServerKeyExchange  = 12
	hsCertificateRequest = 13
	hsServerHelloDone    = 14
	hsClientKeyExchange  = 16
)

const (
	suiteAES128GCMSHA256 = 0x1301
	// The TLS 1.2 suites (RFC 5289): ECDHE, then AES-128-GCM with SHA-256.
	suiteECDHEECDSAAES128GCMSHA256 = 0xc02b
	suiteECDHERSAAES128GCMSHA256   = 0xc02f
	groupSecp256r1                 = 0x0017
	groupX25519                    = 0x001d
	sigECDSAP256SHA256             = 0x0403
	sigRSAPSSSHA256                = 0x0804 // rsa_pss_rsae_sha256
	sigRSAPKCS1SHA256              = 0x0401 // only for certificates
	sigECDSAP384SHA384             = 0x0503 // only for certificates
	maxCerts                       = 4
	maxCookie                      = 256
)

// helloRetry is the random of a HelloRetryRequest (section 4.1.3).
var helloRetry = [32]byte{
	0xcf, 0x21, 0xad, 0x74, 0xe5, 0x9a, 0x61, 0x11, 0xbe, 0x1d, 0x8c, 0x02, 0x1e, 0x65, 0xb8, 0x91,
	0xc2, 0xa2, 0x11, 0x16, 0x7a, 0xbb, 0x8c, 0x5e, 0x07, 0x9e, 0x09, 0xe2, 0xc8, 0xa8, 0x33, 0x9c,
}

// hsState is the state of the handshake that the Conn does not keep.
type hsState struct {
	priv, pubX, pubY [32]byte
	xPriv, xPub      [32]byte
	offer            int // the group of the client key share
	group            int // the group of the server key share
	random           [32]byte
	sessionID        [32]byte
	cookie           [maxCookie]byte
	cookieLen        int
	peerLimit        int  // the record_size_limit of the server, or 0
	fragLimit        int  // the max_fragment_length that the server agreed to, or 0
	offeredPSK       bool // the ClientHello has the ticket of Config.Session
	resumed          bool // the server accepted it
	hsSecret         [sha256.Size]byte
	// shared, early and derived are the secrets of section 7.1. A local
	// array that is sliced moves to the heap.
	shared, early, derived [32]byte
	cHS, sHS               [sha256.Size]byte
	certs                  [maxCerts]x509.Certificate
	nCerts                 int
	// leafN is a copy of the RSA modulus of the leaf: the certificates refer
	// to the handshake buffer, which addTranscript moves.
	leafN [rsa.MaxBits / 8]byte
	leafE uint32
	nLen  int

	// TLS 1.2 (RFC 5246): the version the server chose, its suite and random,
	// whether it agreed to extended_master_secret (RFC 7627) and asked for a
	// client certificate, and the master secret and the key block.
	v12          bool
	suite        int
	serverRandom [32]byte
	ems          bool
	certReq      bool
	master       [48]byte
	keyBlock     [2*keyLen + 2*saltLen]byte
}

// Handshake runs the TLS 1.3 handshake. It does not go on after an error of
// the transport, so use no read deadline or a long one. After the handshake,
// Read goes on after a timeout.
func (c *Conn) Handshake() error {
	if c.handshaked {
		return nil
	}
	if c.closed {
		return ErrClosed
	}
	wb := c.cfg.WriteBuffer
	if wb == 0 {
		wb = 2048
	}
	c.rx = memory.MakeSlice[byte](c.mem, headerLen+maxRecord)
	c.tx = memory.MakeSlice[byte](c.mem, max(wb, 512))
	c.hs = memory.MakeSlice[byte](c.mem, hsBufferSize)
	st := memory.New[hsState](c.mem)
	if c.rx == nil || c.tx == nil || c.hs == nil || st == nil {
		if st != nil {
			memory.Free(c.mem, st)
		}
		c.free()
		c.closed = true
		return ErrNoMemory
	}
	err := c.handshake(st)
	*st = hsState{}
	memory.Free(c.mem, st)
	c.release(&c.hs)
	if err != nil {
		c.free()
		c.closed = true
		return err
	}
	c.handshaked = true
	if c.recvLimit > 0 {
		// The handshake has used all bytes of rx, so a smaller one can take
		// its place. 256 is the most that a TLS 1.3 record adds to the
		// plaintext (RFC 8446 section 5.2).
		if rx := memory.MakeSlice[byte](c.mem, headerLen+c.recvLimit+256); rx != nil {
			c.release(&c.rx)
			c.rx = rx
		}
	}
	return nil
}

func (c *Conn) handshake(st *hsState) error {
	c.transcript.Reset(sha256.Init, 0)
	for _, b := range [][]byte{st.random[:], st.sessionID[:], st.xPriv[:]} {
		if _, err := io.ReadFull(c.rng, b); err != nil {
			return err
		}
	}
	// Only an X25519 key share, and secp256r1 after a HelloRetryRequest.
	x25519.X25519(&st.xPub, &st.xPriv, &x25519.Basepoint)
	st.offer = groupX25519
	if err := c.sendClientHello(st); err != nil {
		return err
	}
	var peerX, peerY [32]byte
	for {
		typ, body, err := c.readHandshake()
		if err != nil {
			return err
		}
		if typ != hsServerHello {
			return ErrHandshake
		}
		retry, err := parseServerHello(body, st, &peerX, &peerY, c.cfg.RecordSizeLimit)
		if err != nil {
			return err
		}
		if !retry {
			c.addTranscript()
			if st.v12 {
				return c.handshake12(st)
			}
			break
		}
		if st.offer != groupX25519 {
			return ErrRetry // a second HelloRetryRequest (section 4.1.4)
		}
		// The transcript starts again with the hash of ClientHello1 in a
		// message_hash message (section 4.4.1).
		h := c.hash()
		c.transcript.Reset(sha256.Init, 0)
		c.transcript.Write([]byte{hsMessageHash, 0, 0, sha256.Size})
		c.transcript.Write(h[:])
		c.addTranscript()
		if c.hsLen != 0 {
			return ErrHandshake
		}
		if err := p256.GenerateKey(c.mem, c.rng, &st.priv, &st.pubX, &st.pubY); err != nil {
			return err
		}
		st.offer = groupSecp256r1
		if err := c.sendClientHello(st); err != nil {
			return err
		}
	}
	var typ byte
	var body []byte
	var err error

	// The handshake secrets (section 7.1).
	shared, early, derived := &st.shared, &st.early, &st.derived
	if st.group == groupX25519 {
		x25519.X25519(shared, &st.xPriv, &peerX)
		if *shared == zeros { // RFC 8446 section 7.4.2
			return ErrHandshake
		}
	} else if err := p256.SharedSecret(c.mem, &st.priv, &peerX, &peerY, shared); err != nil {
		return ErrHandshake
	}
	clear(st.priv[:])
	clear(st.xPriv[:])
	if st.resumed {
		earlySecret(early, c.cfg.Session.psk[:])
	} else {
		earlySecret(early, nil)
	}
	deriveSecret(derived, early[:], "derived", emptyHash[:])
	hkdf.Extract(&st.hsSecret, derived[:], shared[:])
	clear(shared[:])
	th := c.hash()
	deriveSecret(&st.cHS, st.hsSecret[:], "c hs traffic", th[:])
	deriveSecret(&st.sHS, st.hsSecret[:], "s hs traffic", th[:])
	if err := c.read.set(c.mem, st.sHS[:]); err != nil {
		return err
	}

	if typ, body, err = c.readHandshake(); err != nil {
		return err
	}
	if typ != hsEncryptedExtensions {
		return ErrHandshake
	}
	if err := c.parseEncryptedExtensions(body, st); err != nil {
		return err
	}
	c.addTranscript()
	th = c.hash()

	// A resumed session has no certificates (section 2.2).
	if !st.resumed {
		if typ, body, err = c.readHandshake(); err != nil {
			return err
		}
		if typ != hsCertificate {
			return ErrHandshake
		}
		if err := parseCertificates(body, st, false); err != nil {
			return err
		}
		if err := c.checkChain(st); err != nil {
			return err
		}
		if leaf := &st.certs[0]; leaf.RSA {
			st.nLen = copy(st.leafN[:], leaf.RSAKey.N)
			st.leafE = leaf.RSAKey.E
		}
		c.addTranscript()
		th = c.hash()

		if typ, body, err = c.readHandshake(); err != nil {
			return err
		}
		if typ != hsCertificateVerify {
			return ErrHandshake
		}
		if err := c.checkVerify(body, st, th[:]); err != nil {
			return err
		}
		c.addTranscript()
		th = c.hash()
	}

	if typ, body, err = c.readHandshake(); err != nil {
		return err
	}
	var want [sha256.Size]byte
	finished(&want, st.sHS[:], th[:])
	if typ != hsFinished || subtle.ConstantTimeCompare(body, want[:]) != 1 {
		return ErrFinished
	}
	c.addTranscript()
	th = c.hash()
	if c.hsLen != 0 {
		return ErrHandshake // data after Finished in the same key epoch
	}

	// The application secrets, then the client Finished with the handshake keys.
	var master [32]byte
	deriveSecret(derived, st.hsSecret[:], "derived", emptyHash[:])
	hkdf.Extract(&master, derived[:], zeros[:])
	deriveSecret(&c.cAP, master[:], "c ap traffic", th[:])
	deriveSecret(&c.sAP, master[:], "s ap traffic", th[:])
	clear(early[:])
	clear(derived[:])

	ccs := [1]byte{1}
	if err := c.writeRecord(recordCCS, ccs[:]); err != nil {
		return err
	}
	if err := c.write.set(c.mem, st.cHS[:]); err != nil {
		return err
	}
	var fin [4 + sha256.Size]byte
	fin[0], fin[3] = hsFinished, sha256.Size
	var vd [sha256.Size]byte
	finished(&vd, st.cHS[:], th[:])
	copy(fin[4:], vd[:])
	// The resumption secret covers the transcript up to the client Finished.
	c.transcript.Write(fin[:])
	th = c.hash()
	deriveSecret(&c.resMaster, master[:], "res master", th[:])
	clear(master[:])
	c.resumed = st.resumed
	switch {
	case st.peerLimit > 0:
		c.sendLimit = min(st.peerLimit-1, maxPlain)
		c.recvLimit = min(max(c.cfg.RecordSizeLimit, minRecordLimit), maxPlain)
	case st.fragLimit > 0:
		c.sendLimit, c.recvLimit = st.fragLimit, st.fragLimit
	}
	if err := c.writeRecord(recordHandshake, fin[:]); err != nil {
		return err
	}
	if err := c.write.set(c.mem, c.cAP[:]); err != nil {
		return err
	}
	return c.read.set(c.mem, c.sAP[:])
}

// finished is the verify_data: HMAC(finished_key, th) (section 4.4.4).
func finished(out *[sha256.Size]byte, secret, th []byte) {
	var key [sha256.Size]byte
	expandLabel(key[:], secret, "finished", nil)
	var h sha256.HMAC
	h.Init(key[:])
	h.Sum(out, th, nil, nil, nil)
	h.Clear()
	clear(key[:])
}

// hash gives the transcript hash so far.
func (c *Conn) hash() [sha256.Size]byte {
	d := c.transcript
	var out [sha256.Size]byte
	d.Sum(&out)
	return out
}

// put appends p to b but does not grow b, because growth allocates. It gives
// nil when p does not fit, and nil stays nil.
func put(b []byte, p ...byte) []byte {
	n := len(b)
	if b == nil || n+len(p) > cap(b) {
		return nil
	}
	b = b[:n+len(p)]
	copy(b[n:], p)
	return b
}

// puts is put for a string.
func puts(b []byte, s string) []byte {
	n := len(b)
	if b == nil || n+len(s) > cap(b) {
		return nil
	}
	b = b[:n+len(s)]
	copy(b[n:], s)
	return b
}

// sendClientHello writes the ClientHello with the key share of st.offer into
// the transcript and sends it.
func (c *Conn) sendClientHello(st *hsState) error {
	name := c.cfg.ServerName
	b := c.hs[:0]
	b = put(b, hsClientHello, 0, 0, 0, 3, 3)
	b = put(b, st.random[:]...)
	b = put(b, 32)
	b = put(b, st.sessionID[:]...)
	// TLS 1.3's suite, then TLS 1.2's, for a server without 1.3.
	b = put(b, 0, 6, suiteAES128GCMSHA256>>8, suiteAES128GCMSHA256&0xff,
		suiteECDHEECDSAAES128GCMSHA256>>8, suiteECDHEECDSAAES128GCMSHA256&0xff,
		suiteECDHERSAAES128GCMSHA256>>8, suiteECDHERSAAES128GCMSHA256&0xff, 1, 0)
	ext := len(b)
	b = put(b, 0, 0)
	// No SNI for an IP address (RFC 6066 section 3).
	if _, isIP := x509.ParseIPv4(name); name != "" && !isIP {
		n := len(name)
		b = put(b, 0, 0, byte((n+5)>>8), byte(n+5), byte((n+3)>>8), byte(n+3), 0, byte(n>>8), byte(n))
		b = puts(b, name)
	}
	b = put(b, 0, 0x0a, 0, 6, 0, 4, groupX25519>>8, groupX25519&0xff, groupSecp256r1>>8, groupSecp256r1&0xff)
	b = put(b, 0, 0x0d, 0, 10, 0, 8, sigECDSAP256SHA256>>8, sigECDSAP256SHA256&0xff,
		sigRSAPSSSHA256>>8, sigRSAPSSSHA256&0xff, sigRSAPKCS1SHA256>>8, sigRSAPKCS1SHA256&0xff,
		sigECDSAP384SHA384>>8, sigECDSAP384SHA384&0xff)
	b = put(b, 0, 0x2b, 0, 5, 4, 3, 4, 3, 3) // supported_versions: 1.3, then 1.2
	// For TLS 1.2: uncompressed points only (RFC 8422 section 5.1.2), the
	// extended master secret (RFC 7627), and no renegotiation (RFC 5746).
	b = put(b, 0, 0x0b, 0, 2, 1, 0)
	b = put(b, 0, 0x17, 0, 0)
	b = put(b, 0xff, 0x01, 0, 1, 0)
	if l := c.cfg.RecordSizeLimit; l > 0 {
		v := min(max(l, minRecordLimit), maxPlain) + 1 // with the content type (RFC 8449 section 4)
		b = put(b, 0, 0x1c, 0, 2, byte(v>>8), byte(v))
		// Also max_fragment_length, which more servers know (RFC 6066
		// section 4). A server that knows both uses record_size_limit.
		if code := fragCode(l); code > 0 {
			b = put(b, 0, 0x01, 0, 1, code)
		}
	}
	if st.offer == groupX25519 {
		b = put(b, 0, 0x33, 0, 38, 0, 36, groupX25519>>8, groupX25519&0xff, 0, 32)
		b = put(b, st.xPub[:]...)
	} else {
		b = put(b, 0, 0x33, 0, 71, 0, 69, groupSecp256r1>>8, groupSecp256r1&0xff, 0, 65, 4)
		b = put(b, st.pubX[:]...)
		b = put(b, st.pubY[:]...)
	}
	if n := st.cookieLen; n > 0 {
		b = put(b, 0, 0x2c, byte((n+2)>>8), byte(n+2), byte(n>>8), byte(n))
		b = put(b, st.cookie[:n]...)
	}
	// Without psk_key_exchange_modes the server sends no tickets (section
	// 4.6.1). Only psk_dhe_ke, so each connection has an ECDHE key.
	if c.cfg.Session != nil {
		b = put(b, 0, 0x2d, 0, 2, 1, 1)
	}
	if age, ok := c.cfg.Session.usable(name); ok {
		b = c.appendPSK(b, ext, st, age)
	} else if b != nil {
		setLengths(b, ext)
	}
	if b == nil {
		return ErrTooLarge
	}
	c.transcript.Write(b)
	return c.writeRecord(recordHandshake, b)
}

// readHandshake gives the next handshake message. It stays in c.hs until
// addTranscript.
func (c *Conn) readHandshake() (byte, []byte, error) {
	for {
		if c.hsLen >= 4 {
			n := int(c.hs[1])<<16 | int(c.hs[2])<<8 | int(c.hs[3])
			if 4+n > len(c.hs) {
				return 0, nil, ErrTooLarge
			}
			if c.hsLen >= 4+n {
				return c.hs[0], c.hs[4 : 4+n], nil
			}
		}
		typ, content, err := c.readRecord()
		if err != nil {
			return 0, nil, err
		}
		switch typ {
		case recordHandshake:
		case recordAlert:
			return 0, nil, ErrAlert
		default:
			return 0, nil, ErrRecord
		}
		if c.hsLen+len(content) > len(c.hs) {
			return 0, nil, ErrTooLarge
		}
		c.hsLen += copy(c.hs[c.hsLen:], content)
	}
}

// addTranscript adds the current message to the transcript and removes it.
func (c *Conn) addTranscript() {
	n := 4 + (int(c.hs[1])<<16 | int(c.hs[2])<<8 | int(c.hs[3]))
	c.transcript.Write(c.hs[:n])
	copy(c.hs, c.hs[n:c.hsLen])
	c.hsLen -= n
}

// parseServerHello reads a ServerHello, or a HelloRetryRequest, which it
// reports with retry. A HelloRetryRequest must ask for secp256r1, and its
// cookie goes into st (section 4.1.4). A ServerHello with no
// supported_versions is TLS 1.2's, which sets st.v12 (parseServerHello12).
// limit is Config.RecordSizeLimit.
func parseServerHello(b []byte, st *hsState, x, y *[32]byte, limit int) (retry bool, err error) {
	if len(b) < 2+32+1 {
		return false, ErrHandshake
	}
	random := [32]byte(b[2:34])
	retry = random == helloRetry
	sidLen := int(b[34])
	if sidLen > 32 || len(b) < 35+sidLen+3 {
		return false, ErrHandshake
	}
	sameSID := sidLen == 32 && [32]byte(b[35:67]) == st.sessionID
	rest := b[35+sidLen:]
	suite := int(rest[0])<<8 | int(rest[1])
	if rest[2] != 0 {
		return false, ErrUnsupported
	}
	ext := rest[3:]
	if len(ext) > 0 { // a TLS 1.2 ServerHello may have no extensions
		if len(ext) < 2 || int(ext[0])<<8|int(ext[1]) != len(ext)-2 {
			return false, ErrHandshake
		}
		ext = ext[2:]
	}
	sv, ok, err := findExtension(ext, 0x2b) // supported_versions
	if err != nil {
		return false, err
	}
	if !ok {
		if retry {
			return false, ErrUnsupported
		}
		return false, parseServerHello12(b[:2], &random, sameSID, suite, ext, st, limit)
	}
	if len(sv) != 2 || sv[0] != 3 || sv[1] != 4 {
		return false, ErrUnsupported
	}
	if !sameSID {
		return false, ErrHandshake
	}
	if suite != suiteAES128GCMSHA256 {
		return false, ErrUnsupported
	}
	key := false
	st.cookieLen = 0
	for len(ext) >= 4 {
		typ, n := int(ext[0])<<8|int(ext[1]), int(ext[2])<<8|int(ext[3])
		v := ext[4 : 4+n]
		ext = ext[4+n:]
		switch typ {
		case 0x2c: // cookie
			if !retry || n < 2 || int(v[0])<<8|int(v[1]) != n-2 || n-2 > maxCookie {
				return false, ErrUnsupported
			}
			st.cookieLen = copy(st.cookie[:], v[2:])
		case 0x29: // pre_shared_key: the index of the ticket
			if retry || !st.offeredPSK || n != 2 || v[0] != 0 || v[1] != 0 {
				return false, ErrUnsupported
			}
			st.resumed = true
		case 0x33: // key_share
			switch {
			case retry:
				if n != 2 || int(v[0])<<8|int(v[1]) != groupSecp256r1 {
					return false, ErrUnsupported
				}
			case st.offer == groupX25519 && n == 4+32 && int(v[0])<<8|int(v[1]) == groupX25519 && v[2] == 0 && v[3] == 32:
				copy(x[:], v[4:36])
				st.group = groupX25519
			case st.offer == groupSecp256r1 && n == 4+65 && int(v[0])<<8|int(v[1]) == groupSecp256r1 && v[2] == 0 && v[3] == 65 && v[4] == 4:
				copy(x[:], v[5:37])
				copy(y[:], v[37:69])
				st.group = groupSecp256r1
			default:
				return false, ErrUnsupported
			}
			key = true
		}
	}
	if !key {
		return false, ErrUnsupported
	}
	return retry, nil
}

// downgrade is the end of the random of a TLS 1.3 server that answers with
// TLS 1.2, which a client that offered 1.3 refuses (RFC 8446 section 4.1.3):
// someone between has taken 1.3 out of the ClientHello.
var downgrade = [7]byte{'D', 'O', 'W', 'N', 'G', 'R', 'D'}

// parseServerHello12 reads the rest of a TLS 1.2 ServerHello: its version,
// suite and extensions (RFC 5246 section 7.4.1.3).
func parseServerHello12(version []byte, random *[32]byte, sameSID bool, suite int, ext []byte, st *hsState, limit int) error {
	if version[0] != 3 || version[1] != 3 {
		return ErrUnsupported // nothing before 1.2
	}
	if [7]byte(random[24:31]) == downgrade && random[31] <= 1 {
		return ErrDowngrade
	}
	if sameSID {
		return ErrUnsupported // resuming a session this client never had
	}
	if suite != suiteECDHEECDSAAES128GCMSHA256 && suite != suiteECDHERSAAES128GCMSHA256 {
		return ErrUnsupported
	}
	st.v12, st.suite, st.serverRandom = true, suite, *random
	for len(ext) >= 4 {
		typ, n := int(ext[0])<<8|int(ext[1]), int(ext[2])<<8|int(ext[3])
		v := ext[4 : 4+n]
		ext = ext[4+n:]
		switch typ {
		case 0x17: // extended_master_secret
			if n != 0 {
				return ErrHandshake
			}
			st.ems = true
		case 0xff01: // renegotiation_info, empty on a first handshake (RFC 5746)
			if n != 1 || v[0] != 0 {
				return ErrHandshake
			}
		case 0x1c: // record_size_limit (RFC 8449): in the ServerHello in 1.2
			if limit == 0 || n != 2 {
				return ErrUnsupported
			}
			if st.peerLimit = int(v[0])<<8 | int(v[1]); st.peerLimit < minRecordLimit {
				return ErrHandshake
			}
		case 0x01: // max_fragment_length
			code := fragCode(limit)
			if code == 0 || n != 1 || v[0] != code {
				return ErrUnsupported
			}
			st.fragLimit = 1 << (8 + code)
		case 0x29, 0x2c, 0x33: // TLS 1.3's
			return ErrUnsupported
		}
	}
	return nil
}

// findExtension gives the body of the extension typ in ext, and checks that
// every extension's length fits.
func findExtension(ext []byte, typ int) ([]byte, bool, error) {
	var found []byte
	ok := false
	for len(ext) > 0 {
		if len(ext) < 4 {
			return nil, false, ErrHandshake
		}
		t, n := int(ext[0])<<8|int(ext[1]), int(ext[2])<<8|int(ext[3])
		if 4+n > len(ext) {
			return nil, false, ErrHandshake
		}
		if t == typ {
			found, ok = ext[4:4+n], true
		}
		ext = ext[4+n:]
	}
	return found, ok, nil
}

// fragCode gives the max_fragment_length code of the largest size from 512
// to 4096 that is not above limit, or 0.
func fragCode(limit int) byte {
	code := byte(0)
	for c := byte(1); c <= 4; c++ {
		if 1<<(8+c) <= limit {
			code = c
		}
	}
	return code
}

// parseEncryptedExtensions reads record_size_limit and max_fragment_length. The server sends it only
// when the client did, and then sends no record with more plaintext than
// the limit of the client (RFC 8449 section 4).
func (c *Conn) parseEncryptedExtensions(b []byte, st *hsState) error {
	if len(b) < 2 || int(b[0])<<8|int(b[1]) != len(b)-2 {
		return ErrHandshake
	}
	b = b[2:]
	for len(b) >= 4 {
		typ, n := int(b[0])<<8|int(b[1]), int(b[2])<<8|int(b[3])
		if 4+n > len(b) {
			return ErrHandshake
		}
		v := b[4 : 4+n]
		b = b[4+n:]
		switch typ {
		case 0x1c: // record_size_limit
			if c.cfg.RecordSizeLimit == 0 || n != 2 {
				return ErrUnsupported
			}
			if st.peerLimit = int(v[0])<<8 | int(v[1]); st.peerLimit < minRecordLimit {
				return ErrHandshake
			}
		case 0x01: // max_fragment_length: the code of the client
			code := fragCode(c.cfg.RecordSizeLimit)
			if code == 0 || n != 1 || v[0] != code {
				return ErrUnsupported
			}
			st.fragLimit = 1 << (8 + code)
		}
	}
	if len(b) != 0 {
		return ErrHandshake
	}
	return nil
}

// parseCertificates reads the chain of the Certificate message: TLS 1.3's,
// or with v12 TLS 1.2's, which has no request context and no extensions of
// each certificate.
func parseCertificates(b []byte, st *hsState, v12 bool) error {
	if !v12 {
		if len(b) < 1 || b[0] != 0 { // an empty certificate_request_context
			return ErrHandshake
		}
		b = b[1:]
	}
	if len(b) < 3 {
		return ErrHandshake
	}
	list := b[3:]
	if int(b[0])<<16|int(b[1])<<8|int(b[2]) != len(list) {
		return ErrHandshake
	}
	for len(list) > 0 {
		if len(list) < 3 {
			return ErrHandshake
		}
		n := int(list[0])<<16 | int(list[1])<<8 | int(list[2])
		if 3+n > len(list) || !v12 && 3+n+2 > len(list) {
			return ErrHandshake
		}
		der := list[3 : 3+n]
		if v12 {
			list = list[3+n:]
		} else {
			e := int(list[3+n])<<8 | int(list[4+n])
			if 3+n+2+e > len(list) {
				return ErrHandshake
			}
			list = list[3+n+2+e:]
		}
		if st.nCerts == maxCerts {
			continue // the anchor must be in the first certificates
		}
		if x509.Parse(der, &st.certs[st.nCerts]) != nil {
			return ErrCertificate
		}
		st.nCerts++
	}
	if st.nCerts == 0 {
		return ErrCertificate
	}
	return nil
}

// checkChain checks the server name, the validity and the trust of the
// chain.
func (c *Conn) checkChain(st *hsState) error {
	leaf := &st.certs[0]
	if !leaf.P256 && !leaf.RSA {
		return ErrUnsupported
	}
	if !leaf.ForServer() {
		return ErrUsage
	}
	if !leaf.VerifyHostname(c.cfg.ServerName) {
		return ErrName
	}
	var spki [sha256.Size]byte
	var d sha256.Digest
	d.Reset(sha256.Init, 0)
	d.Write(leaf.SPKI)
	d.Sum(&spki)
	for _, pin := range c.cfg.Pins {
		if pin == spki {
			return c.checkTime(leaf)
		}
	}
	for i := 0; i < st.nCerts; i++ {
		cert := &st.certs[i]
		if err := c.checkTime(cert); err != nil {
			return err
		}
		// cert signed the one before it, so it is a CA of the path with i-1
		// CAs below it (RFC 5280 sections 4.2.1.9 and 4.2.1.12).
		if i > 0 && (cert.HasExtKeyUsage && !cert.ServerAuth || cert.MaxPathLen >= 0 && i-1 > cert.MaxPathLen) {
			return ErrUsage
		}
		for k := range c.cfg.Roots {
			r := &c.cfg.Roots[k]
			// A root can sign only certificates that name it as the issuer.
			if r.Subject != nil && !sameBytes(r.Subject, cert.Issuer) {
				continue
			}
			switch {
			case r.P384 != nil:
				if x509.VerifyECDSA384(cert.SigAlg, cert.TBS, cert.Sig, r.P384) {
					return nil
				}
			case r.RSA.N == nil && cert.SigAlg == x509.SigECDSASHA256:
				if x509.VerifyECDSA(c.mem, cert.TBS, cert.Sig, &r.X, &r.Y) {
					return nil
				}
			case r.RSA.N != nil:
				if x509.VerifyRSA(c.mem, cert.SigAlg, cert.TBS, cert.Sig, &r.RSA) {
					return nil
				}
			}
		}
		if i+1 == st.nCerts || !cert.CheckSignatureFrom(c.mem, &st.certs[i+1]) {
			break
		}
	}
	return ErrCertificate
}

func (c *Conn) checkTime(cert *x509.Certificate) error {
	now := c.cfg.Time
	if !now.IsZero() && (now.Before(cert.NotBefore) || now.After(cert.NotAfter)) {
		return ErrExpired
	}
	return nil
}

// checkVerify checks the CertificateVerify signature of the leaf key over
// the transcript hash th (section 4.4.3).
func (c *Conn) checkVerify(b []byte, st *hsState, th []byte) error {
	if len(b) < 4 {
		return ErrHandshake
	}
	alg := int(b[0])<<8 | int(b[1])
	sig := b[4:]
	if int(b[2])<<8|int(b[3]) != len(sig) {
		return ErrHandshake
	}
	const context = "TLS 1.3, server CertificateVerify"
	var msg [64 + len(context) + 1 + sha256.Size]byte
	for i := 0; i < 64; i++ {
		msg[i] = 0x20
	}
	copy(msg[64:], context)
	copy(msg[64+len(context)+1:], th)
	// The certificates refer to moved bytes now. The key arrays and leafN
	// are copies.
	leaf := &st.certs[0]
	switch {
	case alg == sigECDSAP256SHA256 && leaf.P256:
		if !x509.VerifyECDSA(c.mem, msg[:], sig, &leaf.KeyX, &leaf.KeyY) {
			return ErrSignature
		}
	case alg == sigRSAPSSSHA256 && leaf.RSA:
		var d sha256.Digest
		d.Reset(sha256.Init, 0)
		d.Write(msg[:])
		var h [sha256.Size]byte
		d.Sum(&h)
		k := rsa.PublicKey{N: st.leafN[:st.nLen], E: st.leafE}
		if !rsa.VerifyPSS(c.mem, &k, &h, sig) {
			return ErrSignature
		}
	default:
		return ErrUnsupported
	}
	return nil
}

// postHandshake handles the handshake messages after the handshake: it drops
// NewSessionTicket and follows KeyUpdate (section 4.6).
func (c *Conn) postHandshake(b []byte) error {
	for len(b) >= 4 {
		typ, n := b[0], int(b[1])<<16|int(b[2])<<8|int(b[3])
		if 4+n > len(b) {
			return ErrHandshake // a message over two records is not supported here
		}
		body := b[4 : 4+n]
		b = b[4+n:]
		switch typ {
		case hsNewSessionTicket:
			if err := c.storeTicket(body); err != nil {
				return err
			}
		case hsKeyUpdate:
			if n != 1 {
				return ErrHandshake
			}
			next(&c.sAP)
			if err := c.read.set(c.mem, c.sAP[:]); err != nil {
				return err
			}
			if body[0] == 1 { // update_requested
				msg := [5]byte{hsKeyUpdate, 0, 0, 1, 0}
				if err := c.writeRecord(recordHandshake, msg[:]); err != nil {
					return err
				}
				next(&c.cAP)
				if err := c.write.set(c.mem, c.cAP[:]); err != nil {
					return err
				}
			}
		default:
			return ErrHandshake
		}
	}
	if len(b) != 0 {
		return ErrHandshake
	}
	return nil
}

// next is the next application traffic secret (section 7.2).
func next(secret *[sha256.Size]byte) {
	var n [sha256.Size]byte
	expandLabel(n[:], secret[:], "traffic upd", nil)
	*secret = n
	clear(n[:])
}

// sameBytes reports whether a and b hold the same bytes.
func sameBytes(a, b []byte) bool {
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
