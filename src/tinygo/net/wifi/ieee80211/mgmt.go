package ieee80211

import "encoding/binary"

// Capability Information bits (IEEE 802.11-2020 9.4.1.4).
const (
	CapESS           = 0x0001
	CapPrivacy       = 0x0010
	CapShortPreamble = 0x0020
	CapShortSlot     = 0x0400
)

// Authentication algorithm and status codes (IEEE 802.11-2020 9.4.1.1 and table 9-50).
const (
	AuthOpen      = 0
	StatusSuccess = 0
)

// Beacon is a parsed Beacon or Probe Response body (IEEE 802.11-2020 9.3.3.2 and 9.3.3.10).
type Beacon struct {
	Timestamp  uint64
	Interval   uint16 // in time units of 1024 us
	Capability uint16
	Elements   Elements
}

// Auth is a parsed Authentication body (IEEE 802.11-2020 9.3.3.11).
type Auth struct {
	Algorithm uint16
	Seq       uint16
	Status    uint16
	Elements  Elements
}

// AssocResp is a parsed Association Response body (IEEE 802.11-2020 9.3.3.7).
type AssocResp struct {
	Capability uint16
	Status     uint16
	AID        uint16 // bits 14 and 15 are cleared
	Elements   Elements
}

// Mgmt splits a management frame into its header and body.
func Mgmt(frame []byte) (Header, []byte, error) {
	h, err := ParseHeader(frame)
	if err != nil {
		return h, nil, err
	}
	if h.FC.Type() != TypeMgmt {
		return h, nil, ErrNotMgmt
	}
	return h, frame[h.Len:], nil
}

// ParseBeacon decodes a Beacon or Probe Response body.
func ParseBeacon(body []byte) (Beacon, error) {
	var b Beacon
	if len(body) < 12 {
		return b, ErrShort
	}
	b.Timestamp = binary.LittleEndian.Uint64(body)
	b.Interval = binary.LittleEndian.Uint16(body[8:])
	b.Capability = binary.LittleEndian.Uint16(body[10:])
	b.Elements = Elements(body[12:])
	return b, b.Elements.Validate()
}

// ParseAuth decodes an Authentication body.
func ParseAuth(body []byte) (Auth, error) {
	var a Auth
	if len(body) < 6 {
		return a, ErrShort
	}
	a.Algorithm = binary.LittleEndian.Uint16(body)
	a.Seq = binary.LittleEndian.Uint16(body[2:])
	a.Status = binary.LittleEndian.Uint16(body[4:])
	a.Elements = Elements(body[6:])
	return a, a.Elements.Validate()
}

// ParseAssocResp decodes an Association Response body.
func ParseAssocResp(body []byte) (AssocResp, error) {
	var a AssocResp
	if len(body) < 6 {
		return a, ErrShort
	}
	a.Capability = binary.LittleEndian.Uint16(body)
	a.Status = binary.LittleEndian.Uint16(body[2:])
	a.AID = binary.LittleEndian.Uint16(body[4:]) & 0x3fff
	a.Elements = Elements(body[6:])
	return a, a.Elements.Validate()
}

// ParseReason decodes the reason code of a Deauthentication or Disassociation body.
func ParseReason(body []byte) (uint16, error) {
	if len(body) < 2 {
		return 0, ErrShort
	}
	return binary.LittleEndian.Uint16(body), nil
}

// mgmtHeader writes a management header to da from sa with bssid and returns its length.
func mgmtHeader(dst []byte, subtype uint8, da, sa, bssid MAC, seq uint16) (int, error) {
	h := Header{FC: NewFrameControl(TypeMgmt, subtype), Addr1: da, Addr2: sa, Addr3: bssid, SeqCtrl: SeqCtrl(seq, 0)}
	return PutHeader(dst, &h)
}

// PutProbeRequest writes a broadcast Probe Request for ssid (empty for a wildcard) and returns the frame length.
func PutProbeRequest(dst []byte, sa MAC, seq uint16, ssid, rates []byte) (int, error) {
	if len(ssid) > 32 {
		return 0, ErrSSID
	}
	pos, err := mgmtHeader(dst, SubtypeProbeReq, Broadcast, sa, Broadcast, seq)
	if err != nil {
		return 0, err
	}
	if pos, err = AppendElement(dst, pos, EIDSSID, ssid); err != nil {
		return 0, err
	}
	return AppendRates(dst, pos, rates)
}

// PutAuth writes an Open System Authentication frame with transaction sequence 1 and returns the frame length.
func PutAuth(dst []byte, bssid, sa MAC, seq uint16) (int, error) {
	pos, err := mgmtHeader(dst, SubtypeAuth, bssid, sa, bssid, seq)
	if err != nil {
		return 0, err
	}
	if len(dst) < pos+6 {
		return 0, ErrBuffer
	}
	binary.LittleEndian.PutUint16(dst[pos:], AuthOpen)
	binary.LittleEndian.PutUint16(dst[pos+2:], 1)
	binary.LittleEndian.PutUint16(dst[pos+4:], StatusSuccess)
	return pos + 6, nil
}

// PutAssocRequest writes an Association Request with the SSID and rates. rsn is the data of an RSN
// element, or nil for an open network. It returns the frame length.
func PutAssocRequest(dst []byte, bssid, sa MAC, seq, capability, listenInterval uint16, ssid, rates, rsn []byte) (int, error) {
	if len(ssid) > 32 {
		return 0, ErrSSID
	}
	pos, err := mgmtHeader(dst, SubtypeAssocReq, bssid, sa, bssid, seq)
	if err != nil {
		return 0, err
	}
	if len(dst) < pos+4 {
		return 0, ErrBuffer
	}
	binary.LittleEndian.PutUint16(dst[pos:], capability)
	binary.LittleEndian.PutUint16(dst[pos+2:], listenInterval)
	pos += 4
	if pos, err = AppendElement(dst, pos, EIDSSID, ssid); err != nil {
		return 0, err
	}
	if pos, err = AppendRates(dst, pos, rates); err != nil {
		return 0, err
	}
	if rsn != nil {
		if pos, err = AppendElement(dst, pos, EIDRSN, rsn); err != nil {
			return 0, err
		}
	}
	return pos, nil
}

// PutDeauth writes a Deauthentication frame with a reason code and returns the frame length.
func PutDeauth(dst []byte, da, sa, bssid MAC, seq, reason uint16) (int, error) {
	pos, err := mgmtHeader(dst, SubtypeDeauth, da, sa, bssid, seq)
	if err != nil {
		return 0, err
	}
	if len(dst) < pos+2 {
		return 0, ErrBuffer
	}
	binary.LittleEndian.PutUint16(dst[pos:], reason)
	return pos + 2, nil
}
