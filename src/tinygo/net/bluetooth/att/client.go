package att

import (
	"encoding/binary"

	"runtime/memory"
)

const (
	opIndicate           = 0x1d
	codeAttributeNotLong = 0x0b
	transactionMs        = 30000 // ATT transaction timeout (Core spec v5.4 Vol 3 Part F 3.3.3)
)

// Errors of the client.
const (
	ErrBusy       Error = "att: a request is already in progress"
	ErrResponse   Error = "att: server sent an error response, see Client.ErrCode"
	ErrInvalidPDU Error = "att: server sent a response that is not valid"
	ErrTimeout    Error = "att: server did not respond in 30 seconds"
	ErrValueSize  Error = "att: value is longer than ATT_MTU minus 3"
)

// ServiceHandler receives each primary service that discovery finds.
type ServiceHandler interface {
	Service(start, end uint16, uuid UUID)
}

// CharacteristicHandler receives each characteristic declaration that discovery finds.
type CharacteristicHandler interface {
	Characteristic(decl uint16, props uint8, value uint16, uuid UUID)
}

// DescriptorHandler receives each attribute that descriptor discovery finds.
type DescriptorHandler interface {
	Descriptor(handle uint16, uuid UUID)
}

// ReadHandler receives a value in parts. offset is the position of data in the value.
type ReadHandler interface {
	ReadPart(handle, offset uint16, data []byte)
}

// NotificationHandler receives Handle Value Notifications and Indications. The client sends the
// confirmation of an indication after the handler returns.
type NotificationHandler interface {
	Notification(handle uint16, value []byte, indication bool)
}

type procedure uint8

const (
	procNone procedure = iota
	procMTU
	procServices
	procCharacteristics
	procDescriptors
	procRead
	procWrite
)

// Client is the GATT client of one connection. It runs one procedure at a time.
// Route a PDU to it when ForClient reports true, and to the Server otherwise.
type Client struct {
	mem      Memory
	send     Sender
	notify   NotificationHandler
	services ServiceHandler
	chars    CharacteristicHandler
	descs    DescriptorHandler
	reader   ReadHandler
	maxMTU   uint16
	mtu      uint16
	proc     procedure
	op       byte   // opcode of the request in progress
	next     uint16 // next start handle, or next read offset
	end      uint16
	handle   uint16
	code     uint8
	armed    bool
	deadline uint32
	dead     bool
	closed   bool
}

// NewClient returns a client for one connection. maxMTU is the ATT_MTU it offers in ExchangeMTU.
func NewClient(mem Memory, send Sender, notify NotificationHandler, maxMTU uint16) (*Client, error) {
	if mem == nil || send == nil || notify == nil {
		return nil, ErrNilDependency
	}
	return &Client{mem: mem, send: send, notify: notify, maxMTU: max(maxMTU, DefaultMTU), mtu: DefaultMTU}, nil
}

// ForClient reports whether pdu goes to the client. Servers send odd opcodes and clients send even ones (3.4.8).
func ForClient(pdu []byte) bool {
	return len(pdu) > 0 && pdu[0]&opCommandFlag == 0 && pdu[0]&1 == 1
}

// MTU returns the ATT_MTU of the connection.
func (c *Client) MTU() uint16 { return c.mtu }

// Busy reports whether a procedure is in progress.
func (c *Client) Busy() bool { return c.proc != procNone }

// ErrCode returns the ATT error code of the last ErrResponse.
func (c *Client) ErrCode() uint8 { return c.code }

// Close stops the client. It holds no memory between PDUs.
func (c *Client) Close() {
	c.proc = procNone
	c.closed = true
}

// ExchangeMTU offers maxMTU to the server (Vol 3 Part G 4.3.1).
func (c *Client) ExchangeMTU() error {
	var p [2]byte
	binary.LittleEndian.PutUint16(p[:], c.maxMTU)
	return c.request(procMTU, opMTUReq, p[:], nil)
}

// DiscoverServices finds all primary services (Vol 3 Part G 4.4.1).
func (c *Client) DiscoverServices(h ServiceHandler) error {
	c.services = h
	c.next, c.end = 1, 0xffff
	return c.rangeRequest(procServices, opReadByGroupReq, TypePrimaryService)
}

// DiscoverCharacteristics finds all characteristics from start to end, normally one service (Vol 3 Part G 4.6.1).
func (c *Client) DiscoverCharacteristics(start, end uint16, h CharacteristicHandler) error {
	c.chars = h
	c.next, c.end = start, end
	return c.rangeRequest(procCharacteristics, opReadByTypeReq, TypeCharacteristic)
}

// DiscoverDescriptors finds all attributes from start to end (Vol 3 Part G 4.7.1).
func (c *Client) DiscoverDescriptors(start, end uint16, h DescriptorHandler) error {
	c.descs = h
	c.next, c.end = start, end
	return c.rangeRequest(procDescriptors, opFindInfoReq, UUID{})
}

// Read reads a value of any length with Read and Read Blob requests (Vol 3 Part G 4.8.1 and 4.8.3).
func (c *Client) Read(handle uint16, h ReadHandler) error {
	c.reader, c.handle, c.next = h, handle, 0
	var p [2]byte
	binary.LittleEndian.PutUint16(p[:], handle)
	return c.request(procRead, opReadReq, p[:], nil)
}

// Write writes a value with a Write Request and waits for the Write Response (Vol 3 Part G 4.9.3).
func (c *Client) Write(handle uint16, value []byte) error {
	var p [2]byte
	binary.LittleEndian.PutUint16(p[:], handle)
	return c.request(procWrite, opWriteReq, p[:], value)
}

// WriteCommand writes a value with no response (Vol 3 Part G 4.9.1). It can run during a procedure.
func (c *Client) WriteCommand(handle uint16, value []byte) error {
	if err := c.usable(); err != nil {
		return err
	}
	var p [2]byte
	binary.LittleEndian.PutUint16(p[:], handle)
	return c.transmit(opWriteCmd, p[:], value)
}

// Subscribe writes the CCCD at handle to enable notifications, indications, or both (Vol 3 Part G 4.12.3).
func (c *Client) Subscribe(cccd uint16, notify, indicate bool) error {
	var v [2]byte
	if notify {
		v[0] |= 1
	}
	if indicate {
		v[0] |= 2
	}
	return c.Write(cccd, v[:])
}

// Tick ends the procedure when the server does not respond in 30 seconds. After a timeout the
// client stops, because the spec forbids more requests on the bearer (3.3.3). The first Tick after
// a request starts the timer, so the timeout can be late by one Tick interval.
func (c *Client) Tick(now uint32) (done bool, err error) {
	if c.proc == procNone {
		return false, nil
	}
	if !c.armed {
		c.armed, c.deadline = true, now+transactionMs
		return false, nil
	}
	if int32(now-c.deadline) < 0 {
		return false, nil
	}
	c.proc, c.dead = procNone, true
	return true, ErrTimeout
}

// Handle processes one PDU from the server. done is true when the procedure in progress ends,
// and then err is its result. Notifications do not end a procedure.
func (c *Client) Handle(pdu []byte) (done bool, err error) {
	if c.closed {
		return false, ErrClosed
	}
	if len(pdu) == 0 {
		return false, nil
	}
	op, p := pdu[0], pdu[1:]
	if op == opNotify || op == opIndicate {
		if len(p) < 2 {
			return false, ErrInvalidPDU
		}
		c.notify.Notification(binary.LittleEndian.Uint16(p), p[2:], op == opIndicate)
		if op == opIndicate {
			return false, c.transmit(opConfirm, nil, nil)
		}
		return false, nil
	}
	if c.proc == procNone {
		return false, ErrInvalidPDU
	}
	c.armed = false
	if op == opError {
		if len(p) != 4 || p[0] != c.op {
			return c.finish(ErrInvalidPDU)
		}
		return c.errorResponse(p[3])
	}
	if op != c.op+1 {
		return c.finish(ErrInvalidPDU)
	}
	switch c.proc {
	case procMTU:
		if len(p) != 2 {
			return c.finish(ErrInvalidPDU)
		}
		c.mtu = min(c.maxMTU, max(binary.LittleEndian.Uint16(p), DefaultMTU))
		return c.finish(nil)
	case procServices:
		return c.serviceList(p)
	case procCharacteristics:
		return c.characteristics(p)
	case procDescriptors:
		return c.descriptors(p)
	case procRead:
		return c.readPart(p)
	case procWrite:
		if len(p) != 0 {
			return c.finish(ErrInvalidPDU)
		}
		return c.finish(nil)
	}
	return c.finish(ErrInvalidPDU)
}

// errorResponse ends the procedure. Attribute Not Found ends discovery with success (Vol 3 Part G
// 4.4.1), and Attribute Not Long or Invalid Offset ends a long read with success at an offset (4.8.3).
func (c *Client) errorResponse(code uint8) (bool, error) {
	switch {
	case code == CodeAttributeNotFound && (c.proc == procServices || c.proc == procCharacteristics || c.proc == procDescriptors):
		return c.finish(nil)
	case c.proc == procRead && c.op == opReadBlobReq && (code == codeAttributeNotLong || code == CodeInvalidOffset):
		return c.finish(nil)
	}
	c.code = code
	return c.finish(ErrResponse)
}

// serviceList parses a Read By Group Type Response: length, then start, end and UUID per entry.
func (c *Client) serviceList(p []byte) (bool, error) {
	if len(p) < 1 || (p[0] != 6 && p[0] != 20) || (len(p)-1)%int(p[0]) != 0 || len(p) == 1 {
		return c.finish(ErrInvalidPDU)
	}
	size := int(p[0])
	var last uint16
	for e := p[1:]; len(e) > 0; e = e[size:] {
		start, end := binary.LittleEndian.Uint16(e), binary.LittleEndian.Uint16(e[2:])
		uuid, _ := parseUUID(e[4:size])
		if start < c.next || end < start {
			return c.finish(ErrInvalidPDU)
		}
		c.services.Service(start, end, uuid)
		last = end
	}
	return c.advance(last, procServices, opReadByGroupReq, TypePrimaryService)
}

// characteristics parses a Read By Type Response of characteristic declarations: handle, then
// properties, value handle and UUID.
func (c *Client) characteristics(p []byte) (bool, error) {
	if len(p) < 1 || (p[0] != 7 && p[0] != 21) || (len(p)-1)%int(p[0]) != 0 || len(p) == 1 {
		return c.finish(ErrInvalidPDU)
	}
	size := int(p[0])
	var last uint16
	for e := p[1:]; len(e) > 0; e = e[size:] {
		decl, value := binary.LittleEndian.Uint16(e), binary.LittleEndian.Uint16(e[3:])
		uuid, _ := parseUUID(e[5:size])
		if decl < c.next || decl > c.end {
			return c.finish(ErrInvalidPDU)
		}
		c.chars.Characteristic(decl, e[2], value, uuid)
		last = decl
	}
	return c.advance(last, procCharacteristics, opReadByTypeReq, TypeCharacteristic)
}

// descriptors parses a Find Information Response: format 1 has 16-bit UUIDs and format 2 has 128-bit UUIDs.
func (c *Client) descriptors(p []byte) (bool, error) {
	if len(p) < 1 || (p[0] != 1 && p[0] != 2) {
		return c.finish(ErrInvalidPDU)
	}
	size := 4
	if p[0] == 2 {
		size = 18
	}
	if len(p) == 1 || (len(p)-1)%size != 0 {
		return c.finish(ErrInvalidPDU)
	}
	var last uint16
	for e := p[1:]; len(e) > 0; e = e[size:] {
		h := binary.LittleEndian.Uint16(e)
		uuid, _ := parseUUID(e[2:size])
		if h < c.next || h > c.end {
			return c.finish(ErrInvalidPDU)
		}
		c.descs.Descriptor(h, uuid)
		last = h
	}
	return c.advance(last, procDescriptors, opFindInfoReq, UUID{})
}

// readPart gives one part of a value to the reader. A part shorter than ATT_MTU minus 1 is the last.
func (c *Client) readPart(p []byte) (bool, error) {
	c.reader.ReadPart(c.handle, c.next, p)
	full := int(c.mtu) - 1
	if len(p) < full || int(c.next)+len(p) > 0xffff {
		return c.finish(nil)
	}
	c.next += uint16(len(p))
	var q [4]byte
	binary.LittleEndian.PutUint16(q[:], c.handle)
	binary.LittleEndian.PutUint16(q[2:], c.next)
	c.op = opReadBlobReq
	if err := c.transmit(opReadBlobReq, q[:], nil); err != nil {
		return c.finish(err)
	}
	return false, nil
}

// advance sends the next discovery request after last, or ends discovery at the end of the range.
func (c *Client) advance(last uint16, proc procedure, op byte, typ UUID) (bool, error) {
	if last >= c.end {
		return c.finish(nil)
	}
	c.next = last + 1
	c.proc = procNone
	if err := c.rangeRequest(proc, op, typ); err != nil {
		return c.finish(err)
	}
	return false, nil
}

// rangeRequest sends a request with the handle range next to end, and a type when typ is not zero.
func (c *Client) rangeRequest(proc procedure, op byte, typ UUID) error {
	var p [4]byte
	binary.LittleEndian.PutUint16(p[:], c.next)
	binary.LittleEndian.PutUint16(p[2:], c.end)
	if typ == (UUID{}) {
		return c.request(proc, op, p[:], nil)
	}
	var t [16]byte
	n := typ.put(t[:])
	return c.request(proc, op, p[:], t[:n])
}

func (c *Client) usable() error {
	switch {
	case c.closed:
		return ErrClosed
	case c.dead:
		return ErrTimeout
	}
	return nil
}

// request starts a request of a procedure.
func (c *Client) request(proc procedure, op byte, a, b []byte) error {
	if err := c.usable(); err != nil {
		return err
	}
	if c.proc != procNone {
		return ErrBusy
	}
	// Set the state first, because a Sender can deliver the response before it returns.
	c.proc, c.op, c.armed = proc, op, false
	if err := c.transmit(op, a, b); err != nil {
		c.proc = procNone
		return err
	}
	return nil
}

// transmit sends op, a and b as one PDU from a buffer in the injected memory.
func (c *Client) transmit(op byte, a, b []byte) error {
	n := 1 + len(a) + len(b)
	if n > int(c.mtu) {
		return ErrValueSize
	}
	pdu := memory.MakeSlice[byte](c.mem, n)
	if pdu == nil {
		return ErrNoMemory
	}
	pdu[0] = op
	copy(pdu[1+copy(pdu[1:], a):], b)
	err := c.send.SendATT(pdu)
	release(c.mem, pdu)
	return err
}

func (c *Client) finish(err error) (bool, error) {
	c.proc, c.armed = procNone, false
	return true, err
}

// Reset prepares the client for a new connection. It ends a procedure with no result.
func (c *Client) Reset() {
	c.proc, c.armed, c.dead, c.mtu = procNone, false, false, DefaultMTU
}

// SetMTU sets the ATT_MTU after the server of the same connection answers an exchange.
func (c *Client) SetMTU(mtu uint16) { c.mtu = min(max(mtu, DefaultMTU), c.maxMTU) }
