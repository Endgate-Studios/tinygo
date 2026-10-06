package tls

import (
	"crypto/subtle"
	"io"

	"runtime/memory"
	"tinygo/crypto/hkdf"
	"tinygo/crypto/p256"
	"tinygo/crypto/rsa"
	"tinygo/crypto/sha256"
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
	hsKeyUpdate           = 24
)

const (
	suiteAES128GCMSHA256 = 0x1301
	groupSecp256r1       = 0x0017
	sigECDSAP256SHA256   = 0x0403
	sigRSAPSSSHA256      = 0x0804 // rsa_pss_rsae_sha256
	sigRSAPKCS1SHA256    = 0x0401 // only for certificates
	maxCerts             = 4
)

// helloRetry is the random of a HelloRetryRequest (section 4.1.3).
var helloRetry = [32]byte{
	0xcf, 0x21, 0xad, 0x74, 0xe5, 0x9a, 0x61, 0x11, 0xbe, 0x1d, 0x8c, 0x02, 0x1e, 0x65, 0xb8, 0x91,
	0xc2, 0xa2, 0x11, 0x16, 0x7a, 0xbb, 0x8c, 0x5e, 0x07, 0x9e, 0x09, 0xe2, 0xc8, 0xa8, 0x33, 0x9c,
}

// hsState is the state of the handshake that the Conn does not keep.
type hsState struct {
	priv, pubX, pubY [32]byte
	sessionID        [32]byte
	hsSecret         [sha256.Size]byte
	cHS, sHS         [sha256.Size]byte
	certs            [maxCerts]x509.Certificate
	nCerts           int
	// leafN is a copy of the RSA modulus of the leaf: the certificates refer
	// to the handshake buffer, which addTranscript moves.
	leafN [rsa.MaxBits / 8]byte
	leafE uint32
	nLen  int
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
	return nil
}

func (c *Conn) handshake(st *hsState) error {
	c.transcript.Reset(sha256.Init, 0)
	if err := p256.GenerateKey(c.mem, c.rng, &st.priv, &st.pubX, &st.pubY); err != nil {
		return err
	}
	if err := c.sendClientHello(st); err != nil {
		return err
	}
	typ, body, err := c.readHandshake()
	if err != nil {
		return err
	}
	if typ != hsServerHello {
		return ErrHandshake
	}
	var peerX, peerY [32]byte
	if err := parseServerHello(body, st, &peerX, &peerY); err != nil {
		return err
	}
	c.addTranscript()

	// The handshake secrets (section 7.1).
	var shared, early, derived, zero [32]byte
	if err := p256.SharedSecret(c.mem, &st.priv, &peerX, &peerY, &shared); err != nil {
		return ErrHandshake
	}
	clear(st.priv[:])
	hkdf.Extract(&early, nil, zero[:])
	deriveSecret(&derived, early[:], "derived", emptyHash[:])
	hkdf.Extract(&st.hsSecret, derived[:], shared[:])
	clear(shared[:])
	th := c.hash()
	deriveSecret(&st.cHS, st.hsSecret[:], "c hs traffic", th[:])
	deriveSecret(&st.sHS, st.hsSecret[:], "s hs traffic", th[:])
	if err := c.read.set(c.mem, st.sHS[:]); err != nil {
		return err
	}

	if typ, _, err = c.readHandshake(); err != nil {
		return err
	}
	if typ != hsEncryptedExtensions {
		return ErrHandshake
	}
	c.addTranscript()

	if typ, body, err = c.readHandshake(); err != nil {
		return err
	}
	if typ != hsCertificate {
		return ErrHandshake
	}
	if err := parseCertificates(body, st); err != nil {
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
	deriveSecret(&derived, st.hsSecret[:], "derived", emptyHash[:])
	hkdf.Extract(&master, derived[:], zero[:])
	deriveSecret(&c.cAP, master[:], "c ap traffic", th[:])
	deriveSecret(&c.sAP, master[:], "s ap traffic", th[:])
	clear(master[:])
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

// sendClientHello writes the ClientHello into the transcript and sends it.
func (c *Conn) sendClientHello(st *hsState) error {
	var random [32]byte
	if _, err := io.ReadFull(c.rng, random[:]); err != nil {
		return err
	}
	if _, err := io.ReadFull(c.rng, st.sessionID[:]); err != nil {
		return err
	}
	name := c.cfg.ServerName
	b := c.hs[:0]
	b = append(b, hsClientHello, 0, 0, 0, 3, 3)
	b = append(b, random[:]...)
	b = append(b, 32)
	b = append(b, st.sessionID[:]...)
	b = append(b, 0, 2, suiteAES128GCMSHA256>>8, suiteAES128GCMSHA256&0xff, 1, 0)
	ext := len(b)
	b = append(b, 0, 0)
	if name != "" {
		n := len(name)
		b = append(b, 0, 0, byte((n+5)>>8), byte(n+5), byte((n+3)>>8), byte(n+3), 0, byte(n>>8), byte(n))
		b = append(b, name...)
	}
	b = append(b, 0, 0x0a, 0, 4, 0, 2, groupSecp256r1>>8, groupSecp256r1&0xff)
	b = append(b, 0, 0x0d, 0, 8, 0, 6, sigECDSAP256SHA256>>8, sigECDSAP256SHA256&0xff,
		sigRSAPSSSHA256>>8, sigRSAPSSSHA256&0xff, sigRSAPKCS1SHA256>>8, sigRSAPKCS1SHA256&0xff)
	b = append(b, 0, 0x2b, 0, 3, 2, 3, 4)
	b = append(b, 0, 0x33, 0, 71, 0, 69, groupSecp256r1>>8, groupSecp256r1&0xff, 0, 65, 4)
	b = append(b, st.pubX[:]...)
	b = append(b, st.pubY[:]...)
	el := len(b) - ext - 2
	b[ext], b[ext+1] = byte(el>>8), byte(el)
	l := len(b) - 4
	b[1], b[2], b[3] = byte(l>>16), byte(l>>8), byte(l)
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

func parseServerHello(b []byte, st *hsState, x, y *[32]byte) error {
	if len(b) < 2+32+1+32+2+1+2 {
		return ErrHandshake
	}
	if [32]byte(b[2:34]) == helloRetry {
		return ErrRetry
	}
	if b[34] != 32 || [32]byte(b[35:67]) != st.sessionID {
		return ErrHandshake
	}
	if int(b[67])<<8|int(b[68]) != suiteAES128GCMSHA256 || b[69] != 0 {
		return ErrUnsupported
	}
	ext := b[70:]
	if len(ext) < 2 || int(ext[0])<<8|int(ext[1]) != len(ext)-2 {
		return ErrHandshake
	}
	ext = ext[2:]
	version, key := false, false
	for len(ext) >= 4 {
		typ, n := int(ext[0])<<8|int(ext[1]), int(ext[2])<<8|int(ext[3])
		if 4+n > len(ext) {
			return ErrHandshake
		}
		v := ext[4 : 4+n]
		ext = ext[4+n:]
		switch typ {
		case 0x2b: // supported_versions
			version = n == 2 && v[0] == 3 && v[1] == 4
		case 0x33: // key_share
			if n != 4+65 || int(v[0])<<8|int(v[1]) != groupSecp256r1 || v[2] != 0 || v[3] != 65 || v[4] != 4 {
				return ErrUnsupported
			}
			copy(x[:], v[5:37])
			copy(y[:], v[37:69])
			key = true
		}
	}
	if !version || !key || len(ext) != 0 {
		return ErrUnsupported
	}
	return nil
}

// parseCertificates reads the chain of the Certificate message.
func parseCertificates(b []byte, st *hsState) error {
	if len(b) < 4 || b[0] != 0 { // an empty certificate_request_context
		return ErrHandshake
	}
	list := b[4:]
	if int(b[1])<<16|int(b[2])<<8|int(b[3]) != len(list) {
		return ErrHandshake
	}
	for len(list) > 0 {
		if len(list) < 3 {
			return ErrHandshake
		}
		n := int(list[0])<<16 | int(list[1])<<8 | int(list[2])
		if 3+n+2 > len(list) {
			return ErrHandshake
		}
		der := list[3 : 3+n]
		e := int(list[3+n])<<8 | int(list[4+n])
		if 3+n+2+e > len(list) {
			return ErrHandshake
		}
		list = list[3+n+2+e:]
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
		for k := range c.cfg.Roots {
			r := &c.cfg.Roots[k]
			switch {
			case r.RSA.N == nil && cert.SigAlg == x509.SigECDSASHA256:
				if x509.VerifyECDSA(c.mem, cert.TBS, cert.Sig, &r.X, &r.Y) {
					return nil
				}
			case r.RSA.N != nil && cert.SigAlg == x509.SigRSASHA256:
				if x509.VerifyRSA(c.mem, cert.TBS, cert.Sig, &r.RSA) {
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
