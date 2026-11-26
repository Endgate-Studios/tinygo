package l2cap

import "encoding/binary"

// LE signaling command codes (Core spec v5.4 Vol 3 Part A section 4).
const (
	SigCommandReject      = 0x01
	SigConnParamUpdateReq = 0x12
	SigConnParamUpdateRsp = 0x13

	RejectNotUnderstood = 0x0000

	ParamsAccepted = 0x0000
	ParamsRejected = 0x0001
)

// Signal is one signaling command. Data is a view into the frame.
type Signal struct {
	Code       uint8
	Identifier uint8
	Data       []byte
}

// ParseSignal decodes the signaling command in a frame on channel CIDSignaling.
func ParseSignal(payload []byte) (Signal, error) {
	if len(payload) < 4 {
		return Signal{}, ErrShort
	}
	n := int(binary.LittleEndian.Uint16(payload[2:]))
	if len(payload) < 4+n {
		return Signal{}, ErrShort
	}
	return Signal{Code: payload[0], Identifier: payload[1], Data: payload[4 : 4+n]}, nil
}

// PutSignal writes a signaling command into dst and returns its length.
func PutSignal(dst []byte, code, identifier uint8, data []byte) (int, error) {
	n := 4 + len(data)
	if len(dst) < n {
		return 0, ErrShort
	}
	dst[0], dst[1] = code, identifier
	binary.LittleEndian.PutUint16(dst[2:], uint16(len(data)))
	copy(dst[4:], data)
	return n, nil
}

// PutCommandReject writes a Command Reject for a command that is not understood (4.1).
func PutCommandReject(dst []byte, identifier uint8) (int, error) {
	var reason [2]byte
	binary.LittleEndian.PutUint16(reason[:], RejectNotUnderstood)
	return PutSignal(dst, SigCommandReject, identifier, reason[:])
}

// ConnParams is the body of a Connection Parameter Update Request (4.20).
// Intervals are in 1.25 ms units and the timeout in 10 ms units.
type ConnParams struct {
	IntervalMin, IntervalMax uint16
	Latency                  uint16
	Timeout                  uint16
}

// PutConnParamUpdateReq writes a Connection Parameter Update Request, which a peripheral sends.
func PutConnParamUpdateReq(dst []byte, identifier uint8, p ConnParams) (int, error) {
	var d [8]byte
	binary.LittleEndian.PutUint16(d[0:], p.IntervalMin)
	binary.LittleEndian.PutUint16(d[2:], p.IntervalMax)
	binary.LittleEndian.PutUint16(d[4:], p.Latency)
	binary.LittleEndian.PutUint16(d[6:], p.Timeout)
	return PutSignal(dst, SigConnParamUpdateReq, identifier, d[:])
}

// ParseConnParamUpdateReq decodes the body of a Connection Parameter Update Request.
func ParseConnParamUpdateReq(s Signal) (ConnParams, error) {
	if s.Code != SigConnParamUpdateReq || len(s.Data) < 8 {
		return ConnParams{}, ErrShort
	}
	d := s.Data
	return ConnParams{
		IntervalMin: binary.LittleEndian.Uint16(d[0:]),
		IntervalMax: binary.LittleEndian.Uint16(d[2:]),
		Latency:     binary.LittleEndian.Uint16(d[4:]),
		Timeout:     binary.LittleEndian.Uint16(d[6:]),
	}, nil
}

// PutConnParamUpdateRsp writes a Connection Parameter Update Response (4.21) with ParamsAccepted or ParamsRejected.
func PutConnParamUpdateRsp(dst []byte, identifier uint8, result uint16) (int, error) {
	var d [2]byte
	binary.LittleEndian.PutUint16(d[:], result)
	return PutSignal(dst, SigConnParamUpdateRsp, identifier, d[:])
}

// Valid reports whether p is inside the ranges of HCI_LE_Connection_Update (Core spec v5.4 Vol 4
// Part E 7.8.18). The timeout must be longer than (1 + latency) * IntervalMax * 2 in ms.
func (p ConnParams) Valid() bool {
	return p.IntervalMin >= 6 && p.IntervalMin <= p.IntervalMax && p.IntervalMax <= 3200 &&
		p.Latency <= 499 && p.Timeout >= 10 && p.Timeout <= 3200 &&
		uint32(p.Timeout)*4 > (1+uint32(p.Latency))*uint32(p.IntervalMax)
}
