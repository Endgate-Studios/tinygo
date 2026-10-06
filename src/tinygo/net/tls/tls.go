// Package tls is a TLS 1.3 client (RFC 8446) with TLS_AES_128_GCM_SHA256,
// key exchange on X25519, or secp256r1 after a HelloRetryRequest, server certificates with ECDSA P-256 or RSA,
// with SHA-256, and CA keys on P-256, P-384 or RSA. It checks the server name, the validity, the certificate
// chain up to a configured CA key or a pinned server key, and the
// CertificateVerify signature. Its buffers come from an injected allocator.
package tls

import (
	"io"
	"time"

	"runtime/memory"
	"tinygo/crypto/rsa"
	"tinygo/crypto/sha256"
	"tinygo/crypto/x509"
)

// RawConn is the transport, for example an ip.TCPConn.
type RawConn interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	SetReadDeadline(t time.Time)
}

// Memory is where the client allocates its buffers and keys.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNoMemory    Error = "tls: out of memory"
	ErrRecord      Error = "tls: bad record"
	ErrHandshake   Error = "tls: bad handshake message"
	ErrUnsupported Error = "tls: the server chose a parameter that is not supported"
	ErrRetry       Error = "tls: the server sent a second HelloRetryRequest"
	ErrAlert       Error = "tls: the server sent an alert"
	ErrDecrypt     Error = "tls: record decryption failed"
	ErrCertificate Error = "tls: the server certificate is not trusted"
	ErrName        Error = "tls: the certificate is not for the server name"
	ErrUsage       Error = "tls: a certificate does not allow this use"
	ErrExpired     Error = "tls: a certificate is not valid at this time"
	ErrSignature   Error = "tls: the server signature is wrong"
	ErrFinished    Error = "tls: the server Finished is wrong"
	ErrClosed      Error = "tls: connection closed"
	ErrTooLarge    Error = "tls: handshake message larger than the buffer"
)

// Record content types (section 5.1).
const (
	recordCCS       = 20
	recordAlert     = 21
	recordHandshake = 22
	recordData      = 23
)

const (
	headerLen    = 5
	maxPlain     = 16384
	maxRecord    = maxPlain + 256
	tagLen       = 16
	hsBufferSize = 8 << 10
	// minRecordLimit is the smallest record_size_limit (RFC 8449 section 4).
	minRecordLimit = 64
)

// PublicKey is the key of a trusted CA: a P-256 key, an RSA key when RSA.N
// is not nil, or a P-384 key when P384 is not nil.
type PublicKey struct {
	X, Y [32]byte
	RSA  rsa.PublicKey
	P384 []byte // x and y, 48 bytes each
}

// AnchorFromCertificate gives the key of a CA certificate in DER, for
// Config.Roots. An RSA or P-384 key refers to der, so der must stay.
func AnchorFromCertificate(der []byte) (PublicKey, error) {
	var c x509.Certificate
	if err := x509.Parse(der, &c); err != nil {
		return PublicKey{}, err
	}
	switch {
	case c.P256:
		return PublicKey{X: c.KeyX, Y: c.KeyY}, nil
	case c.RSA:
		return PublicKey{RSA: c.RSAKey}, nil
	case c.P384:
		return PublicKey{P384: c.Key384}, nil
	}
	return PublicKey{}, ErrUnsupported
}

// Config is the configuration of a connection.
type Config struct {
	// ServerName is the host name for SNI and for the certificate check.
	ServerName string
	// Time is the current time, for example from SNTP. The zero time skips
	// the validity check of the certificates.
	Time time.Time
	// Roots are the trusted CA keys. A certificate in the chain that one of
	// them signed makes the chain trusted.
	Roots []PublicKey
	// Pins are SHA-256 hashes of trusted SubjectPublicKeyInfo of servers.
	Pins [][sha256.Size]byte
	// WriteBuffer is the size of the send buffer. The default is 2048.
	WriteBuffer int
	// Session, when not nil, keeps the last ticket of the server for the
	// next connection, which then skips the certificate checks.
	Session *Session
	// RecordSizeLimit, when not 0, asks the server for records with at most
	// this much plaintext, from 64 to 16384, with record_size_limit (RFC
	// 8449), and with max_fragment_length (RFC 6066) of 512 to 4096. When
	// the server agrees, the receive buffer gets smaller after the handshake.
	RecordSizeLimit int
}

// Conn is a TLS client connection.
type Conn struct {
	raw    RawConn
	mem    Memory
	rng    io.Reader
	cfg    *Config
	rx, tx []byte
	rxHave int    // the bytes of the current record in rx, kept after a timeout
	hs     []byte // handshake reassembly, during the handshake
	hsLen  int

	read, write trafficKeys
	cAP, sAP    [sha256.Size]byte // the application traffic secrets
	resMaster   [sha256.Size]byte // the resumption master secret
	resumed     bool
	sendLimit   int // the most plaintext in a record to the server, or 0
	recvLimit   int // the most plaintext in a record from the server, or 0

	data       []byte // decrypted application data not yet read
	transcript sha256.Digest
	handshaked bool
	closed     bool
	readErr    error
}

// Client makes a client on raw. rng gives the random values, for example
// the hardware random number generator. Call Handshake first.
func Client(raw RawConn, mem Memory, rng io.Reader, cfg *Config) *Conn {
	return &Conn{raw: raw, mem: mem, rng: rng, cfg: cfg}
}

// RecordLimit gives the record_size_limit that the server agreed to, or 0.
func (c *Conn) RecordLimit() int { return c.recvLimit }

// Resumed reports whether the handshake used the ticket of Config.Session.
func (c *Conn) Resumed() bool { return c.resumed }

// SetReadDeadline sets the deadline of Read.
func (c *Conn) SetReadDeadline(t time.Time) { c.raw.SetReadDeadline(t) }

// Read reads application data.
func (c *Conn) Read(b []byte) (int, error) {
	if !c.handshaked {
		if err := c.Handshake(); err != nil {
			return 0, err
		}
	}
	for len(c.data) == 0 {
		if c.readErr != nil {
			return 0, c.readErr
		}
		typ, content, err := c.readRecord()
		if err != nil {
			return 0, err
		}
		switch typ {
		case recordData:
			c.data = content
		case recordHandshake:
			if err := c.postHandshake(content); err != nil {
				return 0, err
			}
		case recordAlert:
			c.readErr = alertError(content)
		default:
			return 0, ErrRecord
		}
	}
	n := copy(b, c.data)
	c.data = c.data[n:]
	return n, nil
}

// Write sends application data in records.
func (c *Conn) Write(b []byte) (int, error) {
	if !c.handshaked {
		if err := c.Handshake(); err != nil {
			return 0, err
		}
	}
	if c.closed {
		return 0, ErrClosed
	}
	sent := 0
	max := len(c.tx) - headerLen - 1 - tagLen
	for len(b) > 0 {
		n := min(len(b), max, maxPlain)
		if c.sendLimit > 0 {
			n = min(n, c.sendLimit)
		}
		if err := c.writeRecord(recordData, b[:n]); err != nil {
			return sent, err
		}
		sent += n
		b = b[n:]
	}
	return sent, nil
}

// Close sends close_notify and frees the buffers and the keys. The caller
// closes raw.
func (c *Conn) Close() error {
	if c.closed {
		return nil
	}
	var err error
	if c.handshaked {
		alert := [2]byte{1, 0} // warning, close_notify
		err = c.writeRecord(recordAlert, alert[:])
	}
	c.closed = true
	c.free()
	return err
}

func (c *Conn) free() {
	c.read.close()
	c.write.close()
	clear(c.cAP[:])
	clear(c.sAP[:])
	clear(c.resMaster[:])
	c.release(&c.rx)
	c.release(&c.tx)
	c.release(&c.hs)
	c.data = nil
}

func (c *Conn) release(b *[]byte) {
	if *b != nil {
		clear(*b)
		memory.FreeSlice(c.mem, *b)
		*b = nil
	}
}

func alertError(content []byte) error {
	if len(content) == 2 && content[1] == 0 { // close_notify
		return io.EOF
	}
	return ErrAlert
}

// readRecord reads one record. Protected records are decrypted, and the
// content type is the inner type. The content is valid until the next call.
// After an error of the transport, for example a read deadline, the bytes of
// the record so far stay, and the next call goes on with them.
func (c *Conn) readRecord() (byte, []byte, error) {
	for {
		if err := c.fill(headerLen); err != nil {
			return 0, nil, err
		}
		h := c.rx[:headerLen]
		n := int(h[3])<<8 | int(h[4])
		if n > maxRecord || n > len(c.rx)-headerLen {
			return 0, nil, ErrRecord
		}
		if err := c.fill(headerLen + n); err != nil {
			return 0, nil, err
		}
		c.rxHave = 0
		body := c.rx[headerLen : headerLen+n]
		typ := h[0]
		if typ == recordCCS {
			continue // compatibility mode (appendix D.4)
		}
		if !c.read.on {
			return typ, body, nil
		}
		if typ != recordData || n < tagLen+1 {
			return 0, nil, ErrRecord
		}
		nonce := c.read.nonce()
		if c.read.gcm.Open(body, &nonce, body, h) != nil {
			return 0, nil, ErrDecrypt
		}
		c.read.seq++
		plain := body[:n-tagLen]
		if c.recvLimit > 0 && len(plain) > c.recvLimit+1 {
			return 0, nil, ErrRecord // record_overflow (RFC 8449 section 4)
		}
		// The inner type is the last byte that is not zero.
		i := len(plain) - 1
		for i >= 0 && plain[i] == 0 {
			i--
		}
		if i < 0 {
			return 0, nil, ErrRecord
		}
		return plain[i], plain[:i], nil
	}
}

// writeRecord sends one record, protected when the write keys are on.
func (c *Conn) writeRecord(typ byte, data []byte) error {
	b := c.tx
	n := len(data)
	if !c.write.on {
		if headerLen+n > len(b) {
			return ErrTooLarge
		}
		b[0], b[1], b[2], b[3], b[4] = typ, 3, 3, byte(n>>8), byte(n)
		copy(b[headerLen:], data)
		return writeFull(c.raw, b[:headerLen+n])
	}
	total := n + 1 + tagLen
	if headerLen+total > len(b) {
		return ErrTooLarge
	}
	b[0], b[1], b[2], b[3], b[4] = recordData, 3, 3, byte(total>>8), byte(total)
	copy(b[headerLen:], data)
	b[headerLen+n] = typ
	nonce := c.write.nonce()
	c.write.gcm.Seal(b[headerLen:headerLen+total], &nonce, b[headerLen:headerLen+n+1], b[:headerLen])
	c.write.seq++
	return writeFull(c.raw, b[:headerLen+total])
}

// fill reads until rx holds need bytes of the record.
func (c *Conn) fill(need int) error {
	for c.rxHave < need {
		n, err := c.raw.Read(c.rx[c.rxHave:need])
		c.rxHave += n
		if err != nil && c.rxHave < need {
			return err
		}
	}
	return nil
}

func writeFull(w RawConn, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
