package tls

import (
	"time"

	"tinygo/crypto/hkdf"
	"tinygo/crypto/sha256"
)

// Session resumption with tickets and psk_dhe_ke (RFC 8446 sections 2.2,
// 4.2.11 and 4.6.1).

const (
	maxTicket   = 512
	maxLifetime = 7 * 24 * 3600 // seconds (section 4.6.1)
	binderLen   = 2 + 1 + sha256.Size
)

// Session holds a ticket of a server for the next connection to it. Give the
// same Session in Config.Session to each connection. The client stores the
// last ticket of the server in it and uses it in the next handshake, which
// then skips the certificates. The zero value is empty.
type Session struct {
	serverName string
	psk        [sha256.Size]byte
	ticket     [maxTicket]byte
	ticketLen  int
	ageAdd     uint32
	lifetime   time.Duration
	received   time.Time
}

// Clear removes the ticket.
func (s *Session) Clear() { *s = Session{} }

// usable reports whether s has a ticket for name that has not expired, and
// gives its obfuscated age (section 4.2.11.1).
func (s *Session) usable(name string) (uint32, bool) {
	if s == nil || s.ticketLen == 0 || s.serverName != name {
		return 0, false
	}
	age := time.Since(s.received)
	if age < 0 || age >= s.lifetime {
		return 0, false
	}
	return uint32(age/time.Millisecond) + s.ageAdd, true
}

// storeTicket reads a NewSessionTicket (section 4.6.1) into the session of
// the configuration. A ticket that does not fit is ignored.
func (c *Conn) storeTicket(b []byte) error {
	if len(b) < 4+4+1 {
		return ErrHandshake
	}
	lifetime := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	ageAdd := uint32(b[4])<<24 | uint32(b[5])<<16 | uint32(b[6])<<8 | uint32(b[7])
	nl := int(b[8])
	b = b[9:]
	if len(b) < nl+2 {
		return ErrHandshake
	}
	nonce := b[:nl]
	b = b[nl:]
	tl := int(b[0])<<8 | int(b[1])
	b = b[2:]
	if tl == 0 || len(b) < tl+2 {
		return ErrHandshake
	}
	ticket := b[:tl]
	b = b[tl:]
	if int(b[0])<<8|int(b[1]) != len(b)-2 {
		return ErrHandshake
	}
	s := c.cfg.Session
	if s == nil || lifetime == 0 || tl > maxTicket || nl > sha256.Size {
		return nil
	}
	s.Clear()
	s.serverName = c.cfg.ServerName
	expandLabel(s.psk[:], c.resMaster[:], "resumption", nonce)
	s.ticketLen = copy(s.ticket[:], ticket)
	s.ageAdd = ageAdd
	s.lifetime = time.Duration(min(lifetime, maxLifetime)) * time.Second
	s.received = time.Now()
	return nil
}

// appendPSK appends pre_shared_key with the binder, the last extension of
// the ClientHello in b. The binder is an HMAC
// over the transcript and the ClientHello without the binders (section
// 4.2.11.2). It also sets the lengths of the message.
func (c *Conn) appendPSK(b []byte, ext int, st *hsState, age uint32) []byte {
	s := c.cfg.Session
	tl := s.ticketLen
	n := 2 + 2 + tl + 4 + binderLen
	b = put(b, 0, 0x29, byte(n>>8), byte(n), byte((tl+6)>>8), byte(tl+6), byte(tl>>8), byte(tl))
	b = put(b, s.ticket[:tl]...)
	b = put(b, byte(age>>24), byte(age>>16), byte(age>>8), byte(age))
	b = put(b, 0, 1+sha256.Size, sha256.Size)
	b = put(b, zeros[:]...)
	if b == nil {
		return nil
	}
	setLengths(b, ext)

	var early, binderKey, fin, th [sha256.Size]byte
	earlySecret(&early, s.psk[:])
	deriveSecret(&binderKey, early[:], "res binder", emptyHash[:])
	expandLabel(fin[:], binderKey[:], "finished", nil)
	d := c.transcript
	d.Write(b[:len(b)-binderLen])
	d.Sum(&th)
	var h sha256.HMAC
	h.Init(fin[:])
	h.Sum((*[sha256.Size]byte)(b[len(b)-sha256.Size:]), th[:], nil, nil, nil)
	h.Clear()
	clear(early[:])
	clear(binderKey[:])
	clear(fin[:])
	st.offeredPSK = true
	return b
}

// setLengths sets the length of the extensions, which start at ext, and of
// the handshake message in b.
func setLengths(b []byte, ext int) {
	el := len(b) - ext - 2
	b[ext], b[ext+1] = byte(el>>8), byte(el)
	l := len(b) - 4
	b[1], b[2], b[3] = byte(l>>16), byte(l>>8), byte(l)
}

// earlySecret is HKDF-Extract with a zero salt and the PSK, or zeros without
// one.
func earlySecret(out *[sha256.Size]byte, psk []byte) {
	if psk == nil {
		psk = zeros[:]
	}
	hkdf.Extract(out, nil, psk)
}

// zeros is the PSK of a full handshake and the empty binder. A local array
// here is allocated on the heap.
var zeros [sha256.Size]byte
