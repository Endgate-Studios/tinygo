package hci

import "encoding/binary"

// Event codes and LE subevent codes (Core spec v5.4 Vol 4 Part E section 7.7).
const (
	EvtDisconnectionComplete = 0x05
	EvtEncryptionChange      = 0x08
	EvtCommandComplete       = 0x0e
	EvtCommandStatus         = 0x0f
	EvtNumCompletedPackets   = 0x13
	EvtKeyRefreshComplete    = 0x30
	EvtLEMeta                = 0x3e

	SubConnectionComplete = 0x01
	SubAdvertisingReport  = 0x02
	SubConnectionUpdate   = 0x03
	SubLongTermKeyRequest = 0x05
)

// Event is a parsed event packet. Params is a view into the packet.
type Event struct {
	Code   uint8
	Params []byte
}

// ParseEvent decodes an H4 event packet.
func ParseEvent(b []byte) (Event, error) {
	if len(b) < 3 {
		return Event{}, ErrShort
	}
	if b[0] != TypeEvent {
		return Event{}, ErrType
	}
	n := int(b[2])
	if len(b) < 3+n {
		return Event{}, ErrShort
	}
	return Event{Code: b[1], Params: b[3 : 3+n]}, nil
}

// PutEvent writes an H4 event packet, for a controller or for tests.
func PutEvent(dst []byte, code uint8, params []byte) (int, error) {
	if len(params) > 255 {
		return 0, ErrParams
	}
	n := 3 + len(params)
	if len(dst) < n {
		return 0, ErrBuffer
	}
	dst[0], dst[1], dst[2] = TypeEvent, code, byte(len(params))
	copy(dst[3:], params)
	return n, nil
}

// CommandComplete is HCI_Command_Complete (7.7.14). Return[0] is the status for most commands.
type CommandComplete struct {
	NumPackets uint8
	Op         Opcode
	Return     []byte
}

// CommandComplete decodes the event, and ok is false for other events.
func (e Event) CommandComplete() (CommandComplete, bool) {
	if e.Code != EvtCommandComplete || len(e.Params) < 3 {
		return CommandComplete{}, false
	}
	return CommandComplete{NumPackets: e.Params[0], Op: Opcode(binary.LittleEndian.Uint16(e.Params[1:])), Return: e.Params[3:]}, true
}

// CommandStatus is HCI_Command_Status (7.7.15).
type CommandStatus struct {
	Status     uint8
	NumPackets uint8
	Op         Opcode
}

// CommandStatus decodes the event, and ok is false for other events.
func (e Event) CommandStatus() (CommandStatus, bool) {
	if e.Code != EvtCommandStatus || len(e.Params) < 4 {
		return CommandStatus{}, false
	}
	return CommandStatus{Status: e.Params[0], NumPackets: e.Params[1], Op: Opcode(binary.LittleEndian.Uint16(e.Params[2:]))}, true
}

// Disconnection is HCI_Disconnection_Complete (7.7.5).
type Disconnection struct {
	Status uint8
	Handle uint16
	Reason uint8
}

// Disconnection decodes the event, and ok is false for other events.
func (e Event) Disconnection() (Disconnection, bool) {
	if e.Code != EvtDisconnectionComplete || len(e.Params) < 4 {
		return Disconnection{}, false
	}
	return Disconnection{Status: e.Params[0], Handle: binary.LittleEndian.Uint16(e.Params[1:]) & 0x0fff, Reason: e.Params[3]}, true
}

// CompletedPackets iterates HCI_Number_Of_Completed_Packets (7.7.19). Call Next until it returns false.
type CompletedPackets struct {
	params []byte
	i, n   int
}

// CompletedPackets returns the iterator, and ok is false for other events or a bad length.
func (e Event) CompletedPackets() (CompletedPackets, bool) {
	if e.Code != EvtNumCompletedPackets || len(e.Params) < 1 {
		return CompletedPackets{}, false
	}
	n := int(e.Params[0])
	if len(e.Params) < 1+4*n {
		return CompletedPackets{}, false
	}
	return CompletedPackets{params: e.Params[1:], n: n}, true
}

// Next returns the next connection handle and its completed packet count.
func (c *CompletedPackets) Next() (handle, count uint16, ok bool) {
	if c.i >= c.n {
		return 0, 0, false
	}
	handle = binary.LittleEndian.Uint16(c.params[2*c.i:]) & 0x0fff
	count = binary.LittleEndian.Uint16(c.params[2*c.n+2*c.i:])
	c.i++
	return handle, count, true
}

// LEMeta returns the subevent code and its parameters, and ok is false for other events.
func (e Event) LEMeta() (subevent uint8, params []byte, ok bool) {
	if e.Code != EvtLEMeta || len(e.Params) < 1 {
		return 0, nil, false
	}
	return e.Params[0], e.Params[1:], true
}

// Connection is HCI_LE_Connection_Complete (7.7.65.1). Interval is in 1.25 ms and Timeout in 10 ms units.
type Connection struct {
	Status        uint8
	Handle        uint16
	Role          uint8 // 0 central, 1 peripheral
	PeerAddrType  uint8
	PeerAddr      Addr
	Interval      uint16
	Latency       uint16
	Timeout       uint16
	ClockAccuracy uint8
}

// ParseConnection decodes the parameters of an LE Connection Complete subevent.
func ParseConnection(p []byte) (Connection, error) {
	if len(p) < 18 {
		return Connection{}, ErrMalformed
	}
	c := Connection{Status: p[0], Handle: binary.LittleEndian.Uint16(p[1:]) & 0x0fff, Role: p[3], PeerAddrType: p[4]}
	copy(c.PeerAddr[:], p[5:11])
	c.Interval = binary.LittleEndian.Uint16(p[11:])
	c.Latency = binary.LittleEndian.Uint16(p[13:])
	c.Timeout = binary.LittleEndian.Uint16(p[15:])
	c.ClockAccuracy = p[17]
	return c, nil
}

// AdvReport is one report of an LE Advertising Report subevent. Data is a view into the packet.
type AdvReport struct {
	EventType uint8
	AddrType  uint8
	Addr      Addr
	Data      []byte
	RSSI      int8
}

// AdvReports iterates the reports of an LE Advertising Report subevent (7.7.65.2).
// The fields of all reports are in arrays, one field after the other.
type AdvReports struct {
	p    []byte
	n, i int
	data int // offset of the next Data field
}

// ParseAdvReports checks the subevent parameters and returns the iterator.
func ParseAdvReports(p []byte) (AdvReports, error) {
	if len(p) < 1 {
		return AdvReports{}, ErrMalformed
	}
	n := int(p[0])
	fixed := 1 + n*(1+1+6+1)
	if len(p) < fixed+n {
		return AdvReports{}, ErrMalformed
	}
	total := 0
	for i := 0; i < n; i++ {
		total += int(p[1+n*8+i])
	}
	if len(p) < fixed+total+n {
		return AdvReports{}, ErrMalformed
	}
	return AdvReports{p: p, n: n, data: fixed}, nil
}

// Next returns the next report.
func (a *AdvReports) Next() (AdvReport, bool) {
	if a.i >= a.n {
		return AdvReport{}, false
	}
	p, n, i := a.p, a.n, a.i
	r := AdvReport{EventType: p[1+i], AddrType: p[1+n+i]}
	copy(r.Addr[:], p[1+2*n+6*i:])
	dl := int(p[1+8*n+i])
	r.Data = p[a.data : a.data+dl]
	a.data += dl
	total := 0
	for j := 0; j < n; j++ {
		total += int(p[1+8*n+j])
	}
	r.RSSI = int8(p[1+9*n+total+i])
	a.i++
	return r, true
}

// LTKRequest is HCI_LE_Long_Term_Key_Request (7.7.65.5).
type LTKRequest struct {
	Handle uint16
	Rand   uint64
	EDIV   uint16
}

// ParseLTKRequest decodes the parameters of an LE Long Term Key Request subevent.
func ParseLTKRequest(p []byte) (LTKRequest, error) {
	if len(p) < 12 {
		return LTKRequest{}, ErrMalformed
	}
	return LTKRequest{Handle: binary.LittleEndian.Uint16(p) & 0x0fff, Rand: binary.LittleEndian.Uint64(p[2:]), EDIV: binary.LittleEndian.Uint16(p[10:])}, nil
}

// EncryptionChange is HCI_Encryption_Change v1 (7.7.8), or HCI_Encryption_Key_Refresh_Complete
// (7.7.39) with Enabled set. On an LE link Enabled is 1 for AES-CCM.
type EncryptionChange struct {
	Status  uint8
	Handle  uint16
	Enabled bool
}

// EncryptionChange decodes either event, and ok is false for other events.
func (e Event) EncryptionChange() (EncryptionChange, bool) {
	switch {
	case e.Code == EvtEncryptionChange && len(e.Params) >= 4:
		return EncryptionChange{Status: e.Params[0], Handle: binary.LittleEndian.Uint16(e.Params[1:]) & 0x0fff, Enabled: e.Params[3] != 0}, true
	case e.Code == EvtKeyRefreshComplete && len(e.Params) >= 3:
		return EncryptionChange{Status: e.Params[0], Handle: binary.LittleEndian.Uint16(e.Params[1:]) & 0x0fff, Enabled: true}, true
	}
	return EncryptionChange{}, false
}

// ConnectionUpdate is HCI_LE_Connection_Update_Complete (7.7.65.3).
type ConnectionUpdate struct {
	Status   uint8
	Handle   uint16
	Interval uint16
	Latency  uint16
	Timeout  uint16
}

// ParseConnectionUpdate decodes the parameters of an LE Connection Update Complete subevent.
func ParseConnectionUpdate(p []byte) (ConnectionUpdate, error) {
	if len(p) < 9 {
		return ConnectionUpdate{}, ErrMalformed
	}
	return ConnectionUpdate{
		Status:   p[0],
		Handle:   binary.LittleEndian.Uint16(p[1:]) & 0x0fff,
		Interval: binary.LittleEndian.Uint16(p[3:]),
		Latency:  binary.LittleEndian.Uint16(p[5:]),
		Timeout:  binary.LittleEndian.Uint16(p[7:]),
	}, nil
}
