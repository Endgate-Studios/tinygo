// Package mqtt is an MQTT 3.1.1 client (OASIS mqtt-v3.1.1) with QoS 0 and 1.
// The caller gives the receive and send buffers, so the client does not
// allocate for each packet. One goroutine uses a Client: it calls Poll in a
// loop, and Publish and Subscribe read the packets that come before their
// acknowledgement.
package mqtt

import "time"

// Conn is a stream connection, for example an ip.TCPConn.
type Conn interface {
	Read(b []byte) (int, error)
	Write(b []byte) (int, error)
	SetReadDeadline(t time.Time)
}

// Error is a string error type, so error values are constants in flash.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrBuffer   Error = "mqtt: packet does not fit in the buffer"
	ErrProtocol Error = "mqtt: bad packet from the broker"
	ErrRefused  Error = "mqtt: the broker refused the connection"
	ErrSubAck   Error = "mqtt: the broker refused the subscription"
	ErrTimeout  Error = "mqtt: no answer from the broker"
	ErrQoS      Error = "mqtt: QoS must be 0 or 1"
)

// Packet types (MQTT 3.1.1 section 2.2.1), in the high 4 bits.
const (
	typeConnect    = 1
	typeConnAck    = 2
	typePublish    = 3
	typePubAck     = 4
	typeSubscribe  = 8
	typeSubAck     = 9
	typePingReq    = 12
	typePingResp   = 13
	typeDisconnect = 14
)

// answerTime is the longest wait for the rest of a packet and for an
// acknowledgement.
const answerTime = 10 * time.Second

// Config is the configuration of a connection.
type Config struct {
	ClientID           string
	Username, Password string // empty for none
	// KeepAlive is the longest time without a packet to the broker. The
	// client sends PINGREQ in time. Zero turns it off.
	KeepAlive time.Duration
	// KeepSession keeps the subscriptions of an earlier session (Clean
	// Session 0).
	KeepSession bool
}

// Message is a received PUBLISH. Topic and Payload are valid only during the
// call of the handler.
type Message struct {
	Topic   []byte
	Payload []byte
	QoS     byte
	Retain  bool
}

// Client is the client side of one connection.
type Client struct {
	conn      Conn
	rx, tx    []byte
	handler   func(m *Message)
	keepAlive time.Duration
	lastSend  time.Time
	pingSent  time.Time
	nextID    uint16
	wantType  byte // the acknowledgement that a call waits for, or 0
	wantID    uint16
	got       bool
	gotByte   byte // the return code of a CONNACK or a SUBACK
}

// New makes a client on conn. rx must hold the largest packet from the broker
// and tx the largest packet to it. handler gets each received message. It
// runs inside Poll, Publish and Subscribe, so it must not call them.
func New(conn Conn, rx, tx []byte, handler func(m *Message)) *Client {
	return &Client{conn: conn, rx: rx, tx: tx, handler: handler}
}

// Connect sends CONNECT and waits for the CONNACK.
func (c *Client) Connect(cfg Config) error {
	flags := byte(0x02) // Clean Session
	if cfg.KeepSession {
		flags = 0
	}
	n := 10 + 2 + len(cfg.ClientID)
	if cfg.Username != "" {
		flags |= 0x80
		n += 2 + len(cfg.Username)
	}
	if cfg.Password != "" {
		flags |= 0x40
		n += 2 + len(cfg.Password)
	}
	keep := int(cfg.KeepAlive / time.Second)
	if keep > 0xffff {
		keep = 0xffff
	}
	b, err := c.start(typeConnect<<4, n)
	if err != nil {
		return err
	}
	b = putString(b, "MQTT")
	b = append(b, 4, flags, byte(keep>>8), byte(keep))
	b = putString(b, cfg.ClientID)
	if cfg.Username != "" {
		b = putString(b, cfg.Username)
	}
	if cfg.Password != "" {
		b = putString(b, cfg.Password)
	}
	c.keepAlive = time.Duration(keep) * time.Second
	if err := c.send(b); err != nil {
		return err
	}
	if err := c.wait(typeConnAck, 0); err != nil {
		return err
	}
	if c.gotByte != 0 {
		return ErrRefused
	}
	return nil
}

// Publish sends a message. With QoS 1 it waits for the PUBACK.
func (c *Client) Publish(topic string, payload []byte, qos byte, retain bool) error {
	if qos > 1 {
		return ErrQoS
	}
	first := byte(typePublish<<4 | qos<<1)
	if retain {
		first |= 1
	}
	n := 2 + len(topic) + len(payload)
	if qos > 0 {
		n += 2
	}
	b, err := c.start(first, n)
	if err != nil {
		return err
	}
	b = putString(b, topic)
	id := uint16(0)
	if qos > 0 {
		id = c.id()
		b = append(b, byte(id>>8), byte(id))
	}
	b = append(b, payload...)
	if err := c.send(b); err != nil {
		return err
	}
	if qos == 0 {
		return nil
	}
	return c.wait(typePubAck, id)
}

// Subscribe subscribes to a topic filter with QoS 0 or 1 and waits for the
// SUBACK.
func (c *Client) Subscribe(filter string, qos byte) error {
	if qos > 1 {
		return ErrQoS
	}
	b, err := c.start(typeSubscribe<<4|2, 2+2+len(filter)+1)
	if err != nil {
		return err
	}
	id := c.id()
	b = append(b, byte(id>>8), byte(id))
	b = putString(b, filter)
	b = append(b, qos)
	if err := c.send(b); err != nil {
		return err
	}
	if err := c.wait(typeSubAck, id); err != nil {
		return err
	}
	if c.gotByte&0x80 != 0 {
		return ErrSubAck
	}
	return nil
}

// Disconnect sends DISCONNECT. The caller then closes the connection.
func (c *Client) Disconnect() error {
	b, err := c.start(typeDisconnect<<4, 0)
	if err != nil {
		return err
	}
	return c.send(b)
}

// Poll waits up to timeout for one packet from the broker and handles it. It
// sends PINGREQ when the keep alive time needs it, and it gives ErrTimeout
// when the broker does not answer it.
func (c *Client) Poll(timeout time.Duration) error {
	if err := c.keep(); err != nil {
		return err
	}
	if c.keepAlive > 0 {
		timeout = min(timeout, time.Until(c.lastSend.Add(c.keepAlive)))
	}
	err := c.receive(time.Now().Add(max(timeout, time.Millisecond)))
	if isTimeout(err) {
		return nil
	}
	return err
}

// keep sends PINGREQ before the keep alive time ends, and checks that a
// PINGRESP came for the last one.
func (c *Client) keep() error {
	if c.keepAlive == 0 {
		return nil
	}
	now := time.Now()
	if !c.pingSent.IsZero() && now.Sub(c.pingSent) > answerTime {
		return ErrTimeout
	}
	// Send at 3/4 of the keep alive time, so the PINGREQ comes in time.
	if c.pingSent.IsZero() && now.Sub(c.lastSend) >= c.keepAlive*3/4 {
		b, err := c.start(typePingReq<<4, 0)
		if err == nil {
			err = c.send(b)
		}
		if err != nil {
			return err
		}
		c.pingSent = now
	}
	return nil
}

// wait reads packets until the acknowledgement of type typ with id comes.
func (c *Client) wait(typ byte, id uint16) error {
	c.wantType, c.wantID, c.got = typ, id, false
	deadline := time.Now().Add(answerTime)
	for !c.got {
		if time.Now().After(deadline) {
			c.wantType = 0
			return ErrTimeout
		}
		if err := c.receive(deadline); err != nil && !isTimeout(err) {
			c.wantType = 0
			return err
		}
	}
	c.wantType = 0
	return nil
}

// receive reads and handles one packet. Only the first byte waits until
// deadline. The rest of a packet has answerTime.
func (c *Client) receive(deadline time.Time) error {
	c.conn.SetReadDeadline(deadline)
	if err := c.readFull(c.rx[:1]); err != nil {
		return err
	}
	c.conn.SetReadDeadline(time.Now().Add(answerTime))
	first := c.rx[0]
	// The remaining length: up to 4 bytes of 7 bits (section 2.2.3).
	n, shift := 0, 0
	for i := 0; ; i++ {
		var b [1]byte
		if err := c.readFull(b[:]); err != nil {
			return err
		}
		n |= int(b[0]&0x7f) << shift
		if b[0]&0x80 == 0 {
			break
		}
		shift += 7
		if i == 3 {
			return ErrProtocol
		}
	}
	if n > len(c.rx) {
		return ErrBuffer // the rest of the packet stays: close the connection
	}
	p := c.rx[:n]
	if err := c.readFull(p); err != nil {
		return err
	}
	return c.handle(first, p)
}

func (c *Client) handle(first byte, p []byte) error {
	switch first >> 4 {
	case typeConnAck:
		if len(p) != 2 {
			return ErrProtocol
		}
		c.ack(typeConnAck, 0, p[1])
	case typePubAck:
		if len(p) != 2 {
			return ErrProtocol
		}
		c.ack(typePubAck, uint16(p[0])<<8|uint16(p[1]), 0)
	case typeSubAck:
		if len(p) < 3 {
			return ErrProtocol
		}
		c.ack(typeSubAck, uint16(p[0])<<8|uint16(p[1]), p[2])
	case typePingResp:
		c.pingSent = time.Time{}
	case typePublish:
		return c.publish(first, p)
	default:
		return ErrProtocol
	}
	return nil
}

func (c *Client) ack(typ byte, id uint16, code byte) {
	if c.wantType == typ && c.wantID == id {
		c.got, c.gotByte = true, code
	}
}

// publish gives a received PUBLISH to the handler, and answers QoS 1 with
// PUBACK after the handler.
func (c *Client) publish(first byte, p []byte) error {
	qos := first >> 1 & 3
	if qos > 1 || len(p) < 2 {
		return ErrProtocol
	}
	tl := int(p[0])<<8 | int(p[1])
	rest := p[2:]
	if tl > len(rest) {
		return ErrProtocol
	}
	m := Message{Topic: rest[:tl], QoS: qos, Retain: first&1 != 0}
	rest = rest[tl:]
	var id [2]byte
	if qos == 1 {
		if len(rest) < 2 {
			return ErrProtocol
		}
		id = [2]byte{rest[0], rest[1]}
		rest = rest[2:]
	}
	m.Payload = rest
	if c.handler != nil {
		c.handler(&m)
	}
	if qos == 1 {
		b, err := c.start(typePubAck<<4, 2)
		if err != nil {
			return err
		}
		return c.send(append(b, id[0], id[1]))
	}
	return nil
}

// start writes the fixed header of a packet with n bytes after it. It checks
// first that the packet fits in tx, so the appends do not allocate.
func (c *Client) start(first byte, n int) ([]byte, error) {
	header := 2
	for v := n >> 7; v > 0; v >>= 7 {
		header++
	}
	if n > 268435455 || header+n > len(c.tx) {
		return nil, ErrBuffer
	}
	b := append(c.tx[:0], first)
	for {
		d := byte(n & 0x7f)
		n >>= 7
		if n > 0 {
			d |= 0x80
		}
		b = append(b, d)
		if n == 0 {
			break
		}
	}
	return b, nil
}

func (c *Client) send(b []byte) error {
	for len(b) > 0 {
		n, err := c.conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	c.lastSend = time.Now()
	return nil
}

func (c *Client) readFull(b []byte) error {
	for len(b) > 0 {
		n, err := c.conn.Read(b)
		b = b[n:]
		if err != nil && len(b) > 0 {
			return err
		}
	}
	return nil
}

func (c *Client) id() uint16 {
	c.nextID++
	if c.nextID == 0 {
		c.nextID = 1
	}
	return c.nextID
}

func putString(b []byte, s string) []byte {
	b = append(b, byte(len(s)>>8), byte(len(s)))
	return append(b, s...)
}

func isTimeout(err error) bool {
	t, ok := err.(interface{ Timeout() bool })
	return ok && t.Timeout()
}
