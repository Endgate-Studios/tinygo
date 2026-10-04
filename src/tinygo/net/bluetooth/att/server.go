package att

import (
	"bytes"
	"encoding/binary"
	"unsafe"

	"runtime/memory"
)

// Opcodes (Core spec v5.4 Vol 3 Part F section 3.4.8).
const (
	opError          = 0x01
	opMTUReq         = 0x02
	opMTURsp         = 0x03
	opFindInfoReq    = 0x04
	opFindInfoRsp    = 0x05
	opFindByTypeReq  = 0x06
	opFindByTypeRsp  = 0x07
	opReadByTypeReq  = 0x08
	opReadByTypeRsp  = 0x09
	opReadReq        = 0x0a
	opReadRsp        = 0x0b
	opReadBlobReq    = 0x0c
	opReadBlobRsp    = 0x0d
	opReadByGroupReq = 0x10
	opReadByGroupRsp = 0x11
	opWriteReq       = 0x12
	opWriteRsp       = 0x13
	opNotify         = 0x1b
	opConfirm        = 0x1e
	opWriteCmd       = 0x52
	opCommandFlag    = 0x40
)

// ATT error codes (Core spec v5.4 Vol 3 Part F 3.4.1.1).
const (
	CodeInvalidHandle        = 0x01
	CodeReadNotPermitted     = 0x02
	CodeWriteNotPermitted    = 0x03
	CodeInvalidPDU           = 0x04
	CodeInsufficientAuth     = 0x05
	CodeRequestNotSupported  = 0x06
	CodeInvalidOffset        = 0x07
	CodeAttributeNotFound    = 0x0a
	CodeInvalidValueLength   = 0x0d
	CodeInsufficientEncrypt  = 0x0f
	CodeUnsupportedGroupType = 0x10
)

// DefaultMTU is the ATT_MTU before an exchange on an LE link (Core spec v5.4 Vol 3 Part F 3.2.8).
const DefaultMTU = 23

const cccdNotify = 0x0001

// Sender sends one ATT PDU to the client, for example through l2cap.Send on CIDATT.
type Sender interface {
	SendATT(pdu []byte) error
}

// Server is the ATT server of one connection.
type Server struct {
	mem    Memory
	db     *DB
	send   Sender
	maxMTU uint16
	mtu    uint16
	cccd   []uint16
	// encrypted is the link state, and bonded tells whether the peer has a key for this device.
	encrypted bool
	bonded    bool
	closed    bool
}

// NewServer returns a server for one connection. maxMTU is the largest ATT_MTU it accepts.
func NewServer(mem Memory, db *DB, send Sender, maxMTU uint16) (*Server, error) {
	if mem == nil || db == nil || send == nil {
		return nil, ErrNilDependency
	}
	s := &Server{mem: mem, db: db, send: send, maxMTU: max(maxMTU, DefaultMTU), mtu: DefaultMTU}
	if db.cccds > 0 {
		if s.cccd = memory.MakeSlice[uint16](mem, db.cccds); s.cccd == nil {
			return nil, ErrNoMemory
		}
	}
	return s, nil
}

// MTU returns the ATT_MTU of the connection.
func (s *Server) MTU() uint16 { return s.mtu }

// Close frees the connection state.
func (s *Server) Close() {
	if s.cccd != nil {
		memory.FreeSlice(s.mem, s.cccd)
		s.cccd = nil
	}
	s.closed = true
}

// Handle processes one PDU from the client and sends the response, if any.
func (s *Server) Handle(pdu []byte) error {
	if s.closed {
		return ErrClosed
	}
	if len(pdu) == 0 {
		return nil
	}
	rsp := memory.MakeSlice[byte](s.mem, int(s.mtu))
	if rsp == nil {
		return ErrNoMemory
	}
	defer release(s.mem, rsp)
	n := s.respond(pdu[0], pdu[1:], rsp)
	if n == 0 {
		return nil
	}
	return s.send.SendATT(rsp[:n])
}

// Notify sends a Handle Value Notification when the client enabled notifications for the value.
func (s *Server) Notify(handle uint16, value []byte) error {
	if s.closed {
		return ErrClosed
	}
	c := s.db.get(handle + 1)
	if s.db.get(handle) == nil || c == nil || c.cccd < 0 {
		return ErrHandle
	}
	if s.cccd[c.cccd]&cccdNotify == 0 {
		return ErrNotifyOff
	}
	if s.db.get(handle).Perm&PermReadEncrypted != 0 && !s.encrypted {
		return ErrNotEncrypted
	}
	if len(value) > int(s.mtu)-3 {
		return ErrMTU
	}
	pdu := memory.MakeSlice[byte](s.mem, 3+len(value))
	if pdu == nil {
		return ErrNoMemory
	}
	defer release(s.mem, pdu)
	pdu[0] = opNotify
	binary.LittleEndian.PutUint16(pdu[1:], handle)
	copy(pdu[3:], value)
	return s.send.SendATT(pdu)
}

// respond writes the response to one request into rsp and returns its length, or 0 for no response.
func (s *Server) respond(op byte, p, rsp []byte) int {
	switch op {
	case opMTUReq:
		if len(p) != 2 {
			return errorRsp(rsp, op, 0, CodeInvalidPDU)
		}
		client := max(binary.LittleEndian.Uint16(p), DefaultMTU)
		s.mtu = min(client, s.maxMTU)
		rsp[0] = opMTURsp
		binary.LittleEndian.PutUint16(rsp[1:], s.maxMTU)
		return 3
	case opFindInfoReq:
		return s.findInfo(p, rsp)
	case opFindByTypeReq:
		return s.findByType(p, rsp)
	case opReadByTypeReq:
		return s.readByType(p, rsp)
	case opReadByGroupReq:
		return s.readByGroup(p, rsp)
	case opReadReq, opReadBlobReq:
		return s.read(op, p, rsp)
	case opWriteReq, opWriteCmd:
		return s.write(op, p, rsp)
	case opConfirm:
		return 0
	}
	if op&opCommandFlag != 0 {
		return 0 // unknown commands are ignored (3.4.8)
	}
	return errorRsp(rsp, op, 0, CodeRequestNotSupported)
}

func errorRsp(rsp []byte, op byte, handle uint16, code uint8) int {
	rsp[0], rsp[1] = opError, op
	binary.LittleEndian.PutUint16(rsp[2:], handle)
	rsp[4] = code
	return 5
}

// handleRange parses a start and end handle and checks them (3.4.3.1).
func handleRange(p []byte) (start, end uint16, ok bool) {
	if len(p) < 4 {
		return 0, 0, false
	}
	start, end = binary.LittleEndian.Uint16(p), binary.LittleEndian.Uint16(p[2:])
	return start, end, start != 0 && start <= end
}

func (s *Server) findInfo(p, rsp []byte) int {
	start, end, ok := handleRange(p)
	if len(p) != 4 {
		return errorRsp(rsp, opFindInfoReq, 0, CodeInvalidPDU)
	}
	if !ok {
		return errorRsp(rsp, opFindInfoReq, start, CodeInvalidHandle)
	}
	n, size := 2, 0
	for h := start; h <= end; h++ {
		a := s.db.get(h)
		if a == nil {
			break
		}
		l := a.Type.Len()
		if size == 0 {
			size = l
		}
		if l != size || n+2+l > len(rsp) {
			break
		}
		binary.LittleEndian.PutUint16(rsp[n:], h)
		n += 2 + a.Type.put(rsp[n+2:])
		if h == 0xffff {
			break
		}
	}
	if size == 0 {
		return errorRsp(rsp, opFindInfoReq, start, CodeAttributeNotFound)
	}
	rsp[0], rsp[1] = opFindInfoRsp, 1
	if size == 16 {
		rsp[1] = 2
	}
	return n
}

// findByType answers primary service discovery by UUID (Core spec v5.4 Vol 3 Part G 4.4.2).
func (s *Server) findByType(p, rsp []byte) int {
	start, end, ok := handleRange(p)
	if len(p) < 6 {
		return errorRsp(rsp, opFindByTypeReq, 0, CodeInvalidPDU)
	}
	if !ok {
		return errorRsp(rsp, opFindByTypeReq, start, CodeInvalidHandle)
	}
	typ := UUID16(binary.LittleEndian.Uint16(p[4:]))
	value := p[6:]
	n := 1
	for h := start; h <= end && n+4 <= len(rsp); h++ {
		a := s.db.get(h)
		if a == nil {
			break
		}
		if a.Type != typ || !bytes.Equal(a.Value, value) {
			continue
		}
		groupEnd := h
		if typ == TypePrimaryService {
			groupEnd = s.db.groupEnd(h)
		}
		binary.LittleEndian.PutUint16(rsp[n:], h)
		binary.LittleEndian.PutUint16(rsp[n+2:], groupEnd)
		n += 4
		if groupEnd == 0xffff {
			break
		}
	}
	if n == 1 {
		return errorRsp(rsp, opFindByTypeReq, start, CodeAttributeNotFound)
	}
	rsp[0] = opFindByTypeRsp
	return n
}

func (s *Server) readByType(p, rsp []byte) int {
	start, end, ok := handleRange(p)
	typ, okType := parseUUID(p[min(4, len(p)):])
	if len(p) < 4 || !okType {
		return errorRsp(rsp, opReadByTypeReq, 0, CodeInvalidPDU)
	}
	if !ok {
		return errorRsp(rsp, opReadByTypeReq, start, CodeInvalidHandle)
	}
	maxValue := min(len(rsp)-4, 253)
	n, size := 2, -1
	for h := start; h <= end; h++ {
		a := s.db.get(h)
		if a == nil {
			break
		}
		if a.Type != typ {
			continue
		}
		if code := s.access(a, PermRead); code != 0 {
			if size < 0 {
				return errorRsp(rsp, opReadByTypeReq, h, code)
			}
			break
		}
		room := len(rsp) - n - 2
		if room <= 0 || size >= 0 && room < size {
			break
		}
		l, code := s.value(a, 0, rsp[n+2:n+2+min(maxValue, room)])
		if code != 0 {
			if size < 0 {
				return errorRsp(rsp, opReadByTypeReq, h, code)
			}
			break
		}
		if size < 0 {
			size = l
		} else if l != size {
			break
		}
		binary.LittleEndian.PutUint16(rsp[n:], h)
		n += 2 + l
	}
	if size < 0 {
		return errorRsp(rsp, opReadByTypeReq, start, CodeAttributeNotFound)
	}
	rsp[0], rsp[1] = opReadByTypeRsp, byte(2+size)
	return n
}

func (s *Server) readByGroup(p, rsp []byte) int {
	start, end, ok := handleRange(p)
	typ, okType := parseUUID(p[min(4, len(p)):])
	if len(p) < 4 || !okType {
		return errorRsp(rsp, opReadByGroupReq, 0, CodeInvalidPDU)
	}
	if !ok {
		return errorRsp(rsp, opReadByGroupReq, start, CodeInvalidHandle)
	}
	if typ != TypePrimaryService {
		return errorRsp(rsp, opReadByGroupReq, start, CodeUnsupportedGroupType)
	}
	n, size := 2, -1
	for h := start; h <= end; h++ {
		a := s.db.get(h)
		if a == nil {
			break
		}
		if a.Type != typ {
			continue
		}
		l := len(a.Value)
		if size >= 0 && l != size || n+4+l > len(rsp) {
			break
		}
		size = l
		groupEnd := s.db.groupEnd(h)
		binary.LittleEndian.PutUint16(rsp[n:], h)
		binary.LittleEndian.PutUint16(rsp[n+2:], groupEnd)
		n += 4 + copy(rsp[n+4:], a.Value)
		h = groupEnd
	}
	if size < 0 {
		return errorRsp(rsp, opReadByGroupReq, start, CodeAttributeNotFound)
	}
	rsp[0], rsp[1] = opReadByGroupRsp, byte(4+size)
	return n
}

func (s *Server) read(op byte, p, rsp []byte) int {
	want := 2
	if op == opReadBlobReq {
		want = 4
	}
	if len(p) != want {
		return errorRsp(rsp, op, 0, CodeInvalidPDU)
	}
	h := binary.LittleEndian.Uint16(p)
	var offset uint16
	if op == opReadBlobReq {
		offset = binary.LittleEndian.Uint16(p[2:])
	}
	a := s.db.get(h)
	if a == nil {
		return errorRsp(rsp, op, h, CodeInvalidHandle)
	}
	if code := s.access(a, PermRead); code != 0 {
		return errorRsp(rsp, op, h, code)
	}
	l, code := s.value(a, offset, rsp[1:])
	if code != 0 {
		return errorRsp(rsp, op, h, code)
	}
	rsp[0] = op + 1 // the response opcode follows the request opcode
	return 1 + l
}

func (s *Server) write(op byte, p, rsp []byte) int {
	if len(p) < 2 {
		if op == opWriteCmd {
			return 0
		}
		return errorRsp(rsp, op, 0, CodeInvalidPDU)
	}
	h := binary.LittleEndian.Uint16(p)
	value := p[2:]
	code := s.writeValue(h, value)
	if op == opWriteCmd {
		return 0
	}
	if code != 0 {
		return errorRsp(rsp, op, h, code)
	}
	rsp[0] = opWriteRsp
	return 1
}

func (s *Server) writeValue(h uint16, value []byte) uint8 {
	a := s.db.get(h)
	if a == nil {
		return CodeInvalidHandle
	}
	if code := s.access(a, PermWrite); code != 0 {
		return code
	}
	switch {
	case a.cccd >= 0:
		if len(value) != 2 {
			return CodeInvalidValueLength
		}
		s.cccd[a.cccd] = binary.LittleEndian.Uint16(value)
		return 0
	case a.Handler != nil:
		return a.Handler.WriteValue(h, value)
	}
	return CodeWriteNotPermitted
}

// access returns 0 when the client may read (PermRead) or write (PermWrite) a now, or the ATT error.
// With no encryption it asks a bonded peer to encrypt, and other peers to pair (Vol 3 Part C 10.3.1).
func (s *Server) access(a *Attribute, op Perm) uint8 {
	if a.Perm&op == 0 {
		if op == PermRead {
			return CodeReadNotPermitted
		}
		return CodeWriteNotPermitted
	}
	// PermReadEncrypted and PermWriteEncrypted are two bits above PermRead and PermWrite.
	if a.Perm&(op<<2) == 0 || s.encrypted {
		return 0
	}
	if s.bonded {
		return CodeInsufficientEncrypt
	}
	return CodeInsufficientAuth
}

// value reads the value of a from offset into dst and returns its length, or an ATT error code.
func (s *Server) value(a *Attribute, offset uint16, dst []byte) (int, uint8) {
	switch {
	case a.cccd >= 0:
		var v [2]byte
		binary.LittleEndian.PutUint16(v[:], s.cccd[a.cccd])
		return copyFrom(v[:], offset, dst)
	case a.Handler != nil:
		return a.Handler.ReadValue(a.Handle, offset, dst)
	}
	return copyFrom(a.Value, offset, dst)
}

func copyFrom(v []byte, offset uint16, dst []byte) (int, uint8) {
	if int(offset) > len(v) {
		return 0, CodeInvalidOffset
	}
	return copy(dst, v[offset:]), 0
}

func release(mem Memory, b []byte) {
	clear(b)
	mem.Free(unsafe.Pointer(unsafe.SliceData(b)))
}

// Reset prepares the server for a new connection: the ATT_MTU is the default, all CCCDs are off,
// and the link is not encrypted.
func (s *Server) Reset() {
	s.mtu = DefaultMTU
	clear(s.cccd)
	s.encrypted, s.bonded = false, false
}

// SetSecurity sets the link state. bonded is true when the peer has a stored key for this device.
func (s *Server) SetSecurity(encrypted, bonded bool) { s.encrypted, s.bonded = encrypted, bonded }

// SetMTU sets the ATT_MTU after the client of the same connection completes an exchange, because
// the two directions share one value (Core spec v5.4 Vol 3 Part F 3.4.2.2).
func (s *Server) SetMTU(mtu uint16) { s.mtu = min(max(mtu, DefaultMTU), s.maxMTU) }

// CCCDCount returns how many CCCD values a bond store keeps for this database.
func (s *Server) CCCDCount() int { return len(s.cccd) }

// CopyCCCDs copies the CCCD values in database order into dst and returns the count.
func (s *Server) CopyCCCDs(dst []uint16) int { return copy(dst, s.cccd) }

// SetCCCDs sets the CCCD values in database order, for a bonded client that connects again
// (Core spec v5.4 Vol 3 Part G 3.3.3.3).
func (s *Server) SetCCCDs(src []uint16) { copy(s.cccd, src) }
