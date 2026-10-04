package hci

import "encoding/binary"

// Advertising and address types (Core spec v5.4 Vol 4 Part E section 7.8.5).
const (
	AdvInd        = 0x00 // connectable and scannable undirected
	AdvDirectInd  = 0x01
	AdvScanInd    = 0x02
	AdvNonconnInd = 0x03

	AddrPublic = 0x00
	AddrRandom = 0x01

	ChannelsAll = 0x07 // advertising channels 37, 38 and 39

	maxAdvData = 31
)

// AdvParams is HCI_LE_Set_Advertising_Parameters. Intervals are in units of 0.625 ms.
type AdvParams struct {
	IntervalMin, IntervalMax uint16
	Type                     uint8
	OwnAddrType              uint8
	PeerAddrType             uint8
	PeerAddr                 Addr
	ChannelMap               uint8
	FilterPolicy             uint8
}

// PutLESetAdvParams writes HCI_LE_Set_Advertising_Parameters (Core spec v5.4 Vol 4 Part E 7.8.5).
func PutLESetAdvParams(dst []byte, p *AdvParams) (int, error) {
	var b [15]byte
	binary.LittleEndian.PutUint16(b[0:], p.IntervalMin)
	binary.LittleEndian.PutUint16(b[2:], p.IntervalMax)
	b[4], b[5], b[6] = p.Type, p.OwnAddrType, p.PeerAddrType
	copy(b[7:13], p.PeerAddr[:])
	b[13], b[14] = p.ChannelMap, p.FilterPolicy
	return PutCommand(dst, OpLESetAdvParams, b[:])
}

// PutLESetAdvData writes HCI_LE_Set_Advertising_Data, or for scanResp HCI_LE_Set_Scan_Response_Data.
// The 31-byte data field is padded with zeros (Core spec v5.4 Vol 4 Part E 7.8.7 and 7.8.8).
func PutLESetAdvData(dst []byte, scanResp bool, data []byte) (int, error) {
	if len(data) > maxAdvData {
		return 0, ErrParams
	}
	var b [1 + maxAdvData]byte
	b[0] = byte(len(data))
	copy(b[1:], data)
	op := OpLESetAdvData
	if scanResp {
		op = OpLESetScanRespData
	}
	return PutCommand(dst, op, b[:])
}

// PutLESetAdvEnable writes HCI_LE_Set_Advertising_Enable.
func PutLESetAdvEnable(dst []byte, on bool) (int, error) {
	return PutCommand(dst, OpLESetAdvEnable, []byte{boolByte(on)})
}

// PutLESetScanParams writes HCI_LE_Set_Scan_Parameters. Interval and window are in units of 0.625 ms.
func PutLESetScanParams(dst []byte, active bool, interval, window uint16, ownAddrType, filterPolicy uint8) (int, error) {
	var b [7]byte
	b[0] = boolByte(active)
	binary.LittleEndian.PutUint16(b[1:], interval)
	binary.LittleEndian.PutUint16(b[3:], window)
	b[5], b[6] = ownAddrType, filterPolicy
	return PutCommand(dst, OpLESetScanParams, b[:])
}

// PutLESetScanEnable writes HCI_LE_Set_Scan_Enable.
func PutLESetScanEnable(dst []byte, on, filterDuplicates bool) (int, error) {
	return PutCommand(dst, OpLESetScanEnable, []byte{boolByte(on), boolByte(filterDuplicates)})
}

// ConnParams is HCI_LE_Create_Connection (Core spec v5.4 Vol 4 Part E 7.8.12).
// Scan values are in 0.625 ms, connection intervals in 1.25 ms and the timeout in 10 ms units.
type ConnParams struct {
	ScanInterval, ScanWindow    uint16
	FilterPolicy                uint8
	PeerAddrType                uint8
	PeerAddr                    Addr
	OwnAddrType                 uint8
	IntervalMin, IntervalMax    uint16
	Latency, SupervisionTimeout uint16
	MinCELength, MaxCELength    uint16
}

// PutLECreateConnection writes HCI_LE_Create_Connection.
func PutLECreateConnection(dst []byte, p *ConnParams) (int, error) {
	var b [25]byte
	binary.LittleEndian.PutUint16(b[0:], p.ScanInterval)
	binary.LittleEndian.PutUint16(b[2:], p.ScanWindow)
	b[4], b[5] = p.FilterPolicy, p.PeerAddrType
	copy(b[6:12], p.PeerAddr[:])
	b[12] = p.OwnAddrType
	binary.LittleEndian.PutUint16(b[13:], p.IntervalMin)
	binary.LittleEndian.PutUint16(b[15:], p.IntervalMax)
	binary.LittleEndian.PutUint16(b[17:], p.Latency)
	binary.LittleEndian.PutUint16(b[19:], p.SupervisionTimeout)
	binary.LittleEndian.PutUint16(b[21:], p.MinCELength)
	binary.LittleEndian.PutUint16(b[23:], p.MaxCELength)
	return PutCommand(dst, OpLECreateConnection, b[:])
}

// PutLELongTermKeyReply writes HCI_LE_Long_Term_Key_Request_Reply with a 16-byte LTK in wire order.
func PutLELongTermKeyReply(dst []byte, handle uint16, ltk *[16]byte) (int, error) {
	var b [18]byte
	binary.LittleEndian.PutUint16(b[:], handle)
	copy(b[2:], ltk[:])
	n, err := PutCommand(dst, OpLELongTermKeyReply, b[:])
	clear(b[:])
	return n, err
}

func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// PutLELongTermKeyNegReply writes HCI_LE_Long_Term_Key_Request_Negative_Reply, when the host has no key (7.8.26).
func PutLELongTermKeyNegReply(dst []byte, handle uint16) (int, error) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], handle)
	return PutCommand(dst, OpLELongTermKeyNeg, b[:])
}

// PutLEEnableEncryption writes HCI_LE_Enable_Encryption with a 16-byte LTK in wire order (7.8.24).
// Secure Connections keys use rand and ediv 0.
func PutLEEnableEncryption(dst []byte, handle uint16, rand uint64, ediv uint16, ltk *[16]byte) (int, error) {
	var b [28]byte
	binary.LittleEndian.PutUint16(b[:], handle)
	binary.LittleEndian.PutUint64(b[2:], rand)
	binary.LittleEndian.PutUint16(b[10:], ediv)
	copy(b[12:], ltk[:])
	n, err := PutCommand(dst, OpLEEnableEncryption, b[:])
	clear(b[:])
	return n, err
}

// ConnUpdate is HCI_LE_Connection_Update (7.8.18). Intervals are in 1.25 ms units, the timeout in
// 10 ms units and the CE lengths in 0.625 ms units.
type ConnUpdate struct {
	Handle                      uint16
	IntervalMin, IntervalMax    uint16
	Latency, SupervisionTimeout uint16
	MinCELength, MaxCELength    uint16
}

// PutLEConnectionUpdate writes HCI_LE_Connection_Update.
func PutLEConnectionUpdate(dst []byte, p *ConnUpdate) (int, error) {
	var b [14]byte
	for i, v := range [7]uint16{p.Handle, p.IntervalMin, p.IntervalMax, p.Latency, p.SupervisionTimeout, p.MinCELength, p.MaxCELength} {
		binary.LittleEndian.PutUint16(b[2*i:], v)
	}
	return PutCommand(dst, OpLEConnectionUpdate, b[:])
}
