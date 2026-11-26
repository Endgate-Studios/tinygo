package tls

import (
	"crypto/subtle"

	"tinygo/crypto/aes"
	"tinygo/crypto/p256"
	"tinygo/crypto/rsa"
	"tinygo/crypto/sha256"
	"tinygo/crypto/x25519"
	"tinygo/crypto/x509"
)

// TLS 1.2 (RFC 5246), for a server without 1.3: ECDHE on X25519 or
// secp256r1 (RFC 8422), signed by the server's ECDSA P-256 or RSA key, then
// AES-128-GCM (RFC 5288), with the extended master secret when the server
// agrees (RFC 7627). No renegotiation, no resumption, no client certificate:
// a server that asks for one gets an empty list.

const (
	saltLen     = 4  // the implicit part of a GCM nonce (RFC 5288 section 3)
	explicitLen = 8  // the explicit part, which each record carries
	verifyLen   = 12 // the verify_data of Finished
	curveNamed  = 3  // ECParameters.curve_type named_curve (RFC 8422 section 5.4)
)

// handshake12 goes on from the ServerHello of a TLS 1.2 server: its
// Certificate, ServerKeyExchange, a CertificateRequest perhaps and
// ServerHelloDone, then the client's flight and the server's Finished.
func (c *Conn) handshake12(st *hsState) error {
	c.v12 = true
	typ, body, err := c.readHandshake()
	if err != nil {
		return err
	}
	if typ != hsCertificate {
		return ErrHandshake
	}
	if err := parseCertificates(body, st, true); err != nil {
		return err
	}
	if err := c.checkChain(st); err != nil {
		return err
	}
	leaf := &st.certs[0]
	if st.suite == suiteECDHEECDSAAES128GCMSHA256 && !leaf.P256 || st.suite == suiteECDHERSAAES128GCMSHA256 && !leaf.RSA {
		return ErrUnsupported // the suite is for the other kind of key
	}
	if leaf.RSA {
		st.nLen = copy(st.leafN[:], leaf.RSAKey.N)
		st.leafE = leaf.RSAKey.E
	}
	c.addTranscript()

	if typ, body, err = c.readHandshake(); err != nil {
		return err
	}
	if typ != hsServerKeyExchange {
		return ErrHandshake
	}
	var pub [65]byte // the client's public key, for ClientKeyExchange
	pubLen, err := c.keyExchange(body, st, &pub)
	if err != nil {
		return err
	}
	c.addTranscript()

	if typ, body, err = c.readHandshake(); err != nil {
		return err
	}
	if typ == hsCertificateRequest {
		st.certReq = true
		c.addTranscript()
		if typ, body, err = c.readHandshake(); err != nil {
			return err
		}
	}
	if typ != hsServerHelloDone || len(body) != 0 {
		return ErrHandshake
	}
	c.addTranscript()
	if c.hsLen != 0 {
		return ErrHandshake
	}

	// The client's flight: an empty Certificate if one was asked for, its key,
	// ChangeCipherSpec and Finished.
	if st.certReq {
		empty := [7]byte{hsCertificate, 0, 0, 3}
		if err := c.sendHandshake(empty[:]); err != nil {
			return err
		}
	}
	var cke [4 + 1 + 65]byte
	cke[0], cke[3], cke[4] = hsClientKeyExchange, byte(1+pubLen), byte(pubLen)
	copy(cke[5:], pub[:pubLen])
	if err := c.sendHandshake(cke[:5+pubLen]); err != nil {
		return err
	}

	// The master secret (RFC 5246 section 8.1, RFC 7627 section 4), then the
	// keys (section 6.3): the client's key, the server's, then their salts.
	if st.ems {
		th := c.hash() // up to ClientKeyExchange
		prf(st.master[:], st.shared[:], "extended master secret", th[:], nil)
	} else {
		prf(st.master[:], st.shared[:], "master secret", st.random[:], st.serverRandom[:])
	}
	clear(st.shared[:])
	prf(st.keyBlock[:], st.master[:], "key expansion", st.serverRandom[:], st.random[:])
	kb := st.keyBlock[:]

	ccs := [1]byte{1}
	if err := c.writeRecord(recordCCS, ccs[:]); err != nil {
		return err
	}
	if err := c.write.set12(c.mem, kb[:keyLen], kb[2*keyLen:2*keyLen+saltLen]); err != nil {
		return err
	}
	var fin [4 + verifyLen]byte
	fin[0], fin[3] = hsFinished, verifyLen
	th := c.hash()
	prf(fin[4:], st.master[:], "client finished", th[:], nil)
	if err := c.sendHandshake(fin[:]); err != nil {
		return err
	}

	// The server's ChangeCipherSpec, which readRecord passes over, turns its
	// keys on; its Finished is the first record they protect.
	if err := c.read.set12(c.mem, kb[keyLen:2*keyLen], kb[2*keyLen+saltLen:]); err != nil {
		return err
	}
	clear(st.keyBlock[:])
	th = c.hash()
	if typ, body, err = c.readHandshake(); err != nil {
		return err
	}
	var want [verifyLen]byte
	prf(want[:], st.master[:], "server finished", th[:], nil)
	clear(st.master[:])
	if typ != hsFinished || subtle.ConstantTimeCompare(body, want[:]) != 1 {
		return ErrFinished
	}
	c.addTranscript()
	if c.hsLen != 0 {
		return ErrHandshake
	}
	switch {
	case st.peerLimit > 0:
		// In 1.2 the limit counts no content type (RFC 8449 section 4).
		c.sendLimit = min(st.peerLimit, maxPlain)
		c.recvLimit = min(max(c.cfg.RecordSizeLimit, minRecordLimit), maxPlain)
	case st.fragLimit > 0:
		c.sendLimit, c.recvLimit = st.fragLimit, st.fragLimit
	}
	return nil
}

// sendHandshake adds a handshake message of the client to the transcript and
// sends it.
func (c *Conn) sendHandshake(msg []byte) error {
	c.transcript.Write(msg)
	return c.writeRecord(recordHandshake, msg)
}

// keyExchange reads the ServerKeyExchange: the server's ECDHE key, signed with
// the key of its certificate over both randoms (RFC 8422 section 5.4). It
// makes the client's key on the same curve, puts the shared secret, the
// premaster secret, in st.shared, and gives the client's public key in pub.
func (c *Conn) keyExchange(b []byte, st *hsState, pub *[65]byte) (int, error) {
	if len(b) < 4 || b[0] != curveNamed {
		return 0, ErrUnsupported
	}
	group, n := int(b[1])<<8|int(b[2]), int(b[3])
	if 4+n+4 > len(b) {
		return 0, ErrHandshake
	}
	params, point := b[:4+n], b[4:4+n]
	alg := int(b[4+n])<<8 | int(b[5+n])
	sig := b[8+n:]
	if int(b[6+n])<<8|int(b[7+n]) != len(sig) {
		return 0, ErrHandshake
	}

	// The signed message: the client random, the server random and the
	// parameters, at most 32+32+4+65 bytes.
	var msg [64 + 4 + 65]byte
	copy(msg[:], st.random[:])
	copy(msg[32:], st.serverRandom[:])
	if len(params) > len(msg)-64 {
		return 0, ErrUnsupported
	}
	m := msg[:64+copy(msg[64:], params)]
	leaf := &st.certs[0]
	switch {
	case alg == sigECDSAP256SHA256 && leaf.P256:
		if !x509.VerifyECDSA(c.mem, m, sig, &leaf.KeyX, &leaf.KeyY) {
			return 0, ErrSignature
		}
	case (alg == sigRSAPKCS1SHA256 || alg == sigRSAPSSSHA256) && leaf.RSA:
		var d sha256.Digest
		d.Reset(sha256.Init, 0)
		d.Write(m)
		var h [sha256.Size]byte
		d.Sum(&h)
		k := rsa.PublicKey{N: st.leafN[:st.nLen], E: st.leafE}
		ok := false
		if alg == sigRSAPKCS1SHA256 {
			ok = rsa.VerifyPKCS1v15(c.mem, &k, &h, sig)
		} else {
			ok = rsa.VerifyPSS(c.mem, &k, &h, sig)
		}
		if !ok {
			return 0, ErrSignature
		}
	default:
		return 0, ErrUnsupported
	}

	switch {
	case group == groupX25519 && n == 32:
		var peer [32]byte
		copy(peer[:], point)
		x25519.X25519(&st.shared, &st.xPriv, &peer)
		clear(st.xPriv[:])
		if st.shared == zeros {
			return 0, ErrHandshake
		}
		return copy(pub[:], st.xPub[:]), nil
	case group == groupSecp256r1 && n == 65 && point[0] == 4:
		var peerX, peerY [32]byte
		copy(peerX[:], point[1:33])
		copy(peerY[:], point[33:65])
		if err := p256.GenerateKey(c.mem, c.rng, &st.priv, &st.pubX, &st.pubY); err != nil {
			return 0, err
		}
		err := p256.SharedSecret(c.mem, &st.priv, &peerX, &peerY, &st.shared)
		clear(st.priv[:])
		if err != nil {
			return 0, ErrHandshake
		}
		pub[0] = 4
		copy(pub[1:], st.pubX[:])
		copy(pub[33:], st.pubY[:])
		return 65, nil
	}
	return 0, ErrUnsupported
}

// prf is TLS 1.2's PRF with SHA-256 (RFC 5246 section 5): P_SHA256 of secret
// over label and the seed, which comes in two parts, into out.
func prf(out, secret []byte, label string, seed1, seed2 []byte) {
	var l [32]byte // the longest label is "extended master secret"
	lb := l[:copy(l[:], label)]
	var h sha256.HMAC
	h.Init(secret)
	var a, block [sha256.Size]byte
	h.Sum(&a, lb, seed1, seed2, nil) // A(1)
	for len(out) > 0 {
		h.Sum(&block, a[:], lb, seed1, seed2)
		out = out[copy(out, block[:]):]
		h.Sum(&a, a[:], nil, nil, nil) // A(i+1)
	}
	h.Clear()
	clear(a[:])
	clear(block[:])
}

// set12 starts the keys of one direction of TLS 1.2: AES-128-GCM with key,
// and a nonce of salt then the 8-byte explicit part (RFC 5288 section 3).
// The IV is the salt then zeros, so nonce gives salt then the sequence
// number, which is what this client sends as its explicit part.
func (t *trafficKeys) set12(mem aes.Memory, key, salt []byte) error {
	t.close()
	copy(t.iv[:], salt)
	if err := t.cipher.Init(mem, key); err != nil {
		return err
	}
	t.gcm.Init(&t.cipher)
	t.seq, t.on = 0, true
	return nil
}

// aad12 is the additional data of a TLS 1.2 record: its sequence number,
// type, version and plaintext length (RFC 5246 section 6.2.3.3).
func aad12(seq uint64, typ byte, n int) [13]byte {
	var a [13]byte
	for i := 0; i < 8; i++ {
		a[7-i] = byte(seq >> (8 * i))
	}
	a[8], a[9], a[10], a[11], a[12] = typ, 3, 3, byte(n>>8), byte(n)
	return a
}

// open12 decrypts a TLS 1.2 record: its explicit nonce, then the ciphertext
// and the tag. The type is the record's own.
func (c *Conn) open12(h, body []byte) (byte, []byte, error) {
	n := len(body) - explicitLen - tagLen
	if n < 0 {
		return 0, nil, ErrRecord
	}
	typ := h[0]
	nonce := c.read.iv
	copy(nonce[saltLen:], body[:explicitLen])
	aad := aad12(c.read.seq, typ, n)
	sealed := body[explicitLen:]
	if c.read.gcm.Open(sealed, &nonce, sealed, aad[:]) != nil {
		return 0, nil, ErrDecrypt
	}
	c.read.seq++
	plain := sealed[:n]
	if c.recvLimit > 0 && n > c.recvLimit+1 {
		return 0, nil, ErrRecord // record_overflow
	}
	return typ, plain, nil
}

// seal12 sends a TLS 1.2 record: the sequence number as its explicit nonce,
// then the ciphertext and the tag.
func (c *Conn) seal12(typ byte, data []byte) error {
	b := c.tx
	n := len(data)
	total := explicitLen + n + tagLen
	if headerLen+total > len(b) {
		return ErrTooLarge
	}
	b[0], b[1], b[2], b[3], b[4] = typ, 3, 3, byte(total>>8), byte(total)
	nonce := c.write.nonce() // the salt, then the sequence number
	copy(b[headerLen:], nonce[saltLen:])
	p := b[headerLen+explicitLen:]
	copy(p, data)
	aad := aad12(c.write.seq, typ, n)
	c.write.gcm.Seal(p[:n+tagLen], &nonce, p[:n], aad[:])
	c.write.seq++
	return writeFull(c.raw, b[:headerLen+total])
}

// helloRequests reports whether b is only HelloRequest messages, which a
// TLS 1.2 server sends to ask for a renegotiation.
func helloRequests(b []byte) bool {
	for len(b) >= 4 {
		if b[0] != hsHelloRequest || b[1] != 0 || b[2] != 0 || b[3] != 0 {
			return false
		}
		b = b[4:]
	}
	return len(b) == 0
}
