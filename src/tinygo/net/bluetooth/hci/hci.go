// Package hci encodes and decodes Bluetooth HCI packets with H4 framing (Core spec v5.4 Vol 4 Parts A and E).
// It writes into caller buffers and parses into views of them. Its own state is on the stack.
package hci

import "encoding/binary"

// H4 packet indicators (Core spec v5.4 Vol 4 Part A section 2).
const (
	TypeCommand = 0x01
	TypeACL     = 0x02
	TypeSCO     = 0x03
	TypeEvent   = 0x04
	TypeISO     = 0x05
)

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrShort     Error = "hci: packet is too short"
	ErrBuffer    Error = "hci: destination buffer is too small"
	ErrType      Error = "hci: wrong packet type"
	ErrParams    Error = "hci: parameters are too long"
	ErrEvent     Error = "hci: wrong event"
	ErrMalformed Error = "hci: event parameters are malformed"
)

// Addr is a Bluetooth device address in wire order, least significant byte first.
type Addr [6]byte

// Opcode is an HCI command opcode: OGF in bits 10 to 15 and OCF in bits 0 to 9.
type Opcode uint16

// NewOpcode returns the opcode for a command group and command.
func NewOpcode(ogf uint8, ocf uint16) Opcode { return Opcode(uint16(ogf)<<10 | ocf&0x3ff) }

func (o Opcode) OGF() uint8  { return uint8(o >> 10) }
func (o Opcode) OCF() uint16 { return uint16(o) & 0x3ff }

// Opcodes that an LE host uses (Core spec v5.4 Vol 4 Part E sections 7.1, 7.3, 7.4 and 7.8).
const (
	OpDisconnect         Opcode = 0x0406
	OpSetEventMask       Opcode = 0x0c01
	OpReset              Opcode = 0x0c03
	OpReadBDAddr         Opcode = 0x1009
	OpLESetEventMask     Opcode = 0x2001
	OpLEReadBufferSize   Opcode = 0x2002
	OpLESetRandomAddress Opcode = 0x2005
	OpLESetAdvParams     Opcode = 0x2006
	OpLESetAdvData       Opcode = 0x2008
	OpLESetScanRespData  Opcode = 0x2009
	OpLESetAdvEnable     Opcode = 0x200a
	OpLESetScanParams    Opcode = 0x200b
	OpLESetScanEnable    Opcode = 0x200c
	OpLECreateConnection Opcode = 0x200d
	OpLELongTermKeyReply Opcode = 0x201a
	OpLELongTermKeyNeg   Opcode = 0x201b
	OpLEEnableEncryption Opcode = 0x2019
	OpLEConnectionUpdate Opcode = 0x2013
)

// PutCommand writes an H4 command packet and returns its length.
func PutCommand(dst []byte, op Opcode, params []byte) (int, error) {
	if len(params) > 255 {
		return 0, ErrParams
	}
	n := 4 + len(params)
	if len(dst) < n {
		return 0, ErrBuffer
	}
	dst[0] = TypeCommand
	binary.LittleEndian.PutUint16(dst[1:], uint16(op))
	dst[3] = byte(len(params))
	copy(dst[4:], params)
	return n, nil
}

// Command is a parsed command packet without the H4 indicator.
type Command struct {
	Op     Opcode
	Params []byte
}

// ParseCommand decodes an H4 command packet, for example on the controller side or in tests.
func ParseCommand(b []byte) (Command, error) {
	if len(b) < 4 {
		return Command{}, ErrShort
	}
	if b[0] != TypeCommand {
		return Command{}, ErrType
	}
	n := int(b[3])
	if len(b) < 4+n {
		return Command{}, ErrShort
	}
	return Command{Op: Opcode(binary.LittleEndian.Uint16(b[1:])), Params: b[4 : 4+n]}, nil
}

// PutReset writes HCI_Reset.
func PutReset(dst []byte) (int, error) { return PutCommand(dst, OpReset, nil) }

// PutSetEventMask writes HCI_Set_Event_Mask or, for le true, HCI_LE_Set_Event_Mask.
func PutSetEventMask(dst []byte, le bool, mask uint64) (int, error) {
	var p [8]byte
	binary.LittleEndian.PutUint64(p[:], mask)
	op := OpSetEventMask
	if le {
		op = OpLESetEventMask
	}
	return PutCommand(dst, op, p[:])
}

// PutDisconnect writes HCI_Disconnect with a reason code, 0x13 for remote user terminated.
func PutDisconnect(dst []byte, handle uint16, reason uint8) (int, error) {
	var p [3]byte
	binary.LittleEndian.PutUint16(p[:], handle)
	p[2] = reason
	return PutCommand(dst, OpDisconnect, p[:])
}
