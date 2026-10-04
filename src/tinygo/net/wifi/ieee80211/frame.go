// Package ieee80211 encodes and decodes IEEE 802.11 frames for a station.
// Parsing returns views into the caller buffer, and encoding writes into a caller buffer. Its own state is on the stack.
//
// Not used over the air yet: espradio exports no raw 802.11, so the Espressif library does this work
// (espradio/DESIGN.md, Wi-Fi radio). It stays for a radio link that gives raw 802.11 frames.
package ieee80211

import "encoding/binary"

// MAC is an IEEE 802 address.
type MAC [6]byte

// Broadcast is the broadcast address.
var Broadcast = MAC{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrShort   Error = "ieee80211: frame is too short"
	ErrBuffer  Error = "ieee80211: destination buffer is too small"
	ErrNotMgmt Error = "ieee80211: frame is not a management frame"
	ErrNotData Error = "ieee80211: frame is not a data frame"
	ErrSubtype Error = "ieee80211: wrong frame subtype"
	ErrIE      Error = "ieee80211: element runs past the end of the frame"
	ErrSSID    Error = "ieee80211: SSID must be at most 32 bytes"
	ErrRSN     Error = "ieee80211: RSN element is malformed"
	ErrSNAP    Error = "ieee80211: data frame has no LLC/SNAP header"
	ErrOrder   Error = "ieee80211: frames with an HT Control field are not supported"
	ErrTooMany Error = "ieee80211: too many rates or suites"
)

// Frame types and subtypes (IEEE 802.11-2020 table 9-1).
const (
	TypeMgmt = 0
	TypeCtrl = 1
	TypeData = 2

	SubtypeAssocReq  = 0
	SubtypeAssocResp = 1
	SubtypeProbeReq  = 4
	SubtypeProbeResp = 5
	SubtypeBeacon    = 8
	SubtypeDisassoc  = 10
	SubtypeAuth      = 11
	SubtypeDeauth    = 12
	SubtypeAction    = 13

	SubtypeData    = 0
	SubtypeQoSData = 8
	SubtypeNull    = 4
	SubtypeQoSNull = 12
)

// FrameControl is the Frame Control field (IEEE 802.11-2020 9.2.4.1), little endian.
type FrameControl uint16

// NewFrameControl returns a Frame Control field with protocol version 0.
func NewFrameControl(typ, subtype uint8) FrameControl {
	return FrameControl(uint16(typ&3)<<2 | uint16(subtype&15)<<4)
}

func (fc FrameControl) Type() uint8     { return uint8(fc>>2) & 3 }
func (fc FrameControl) Subtype() uint8  { return uint8(fc>>4) & 15 }
func (fc FrameControl) ToDS() bool      { return fc&0x0100 != 0 }
func (fc FrameControl) FromDS() bool    { return fc&0x0200 != 0 }
func (fc FrameControl) MoreFrag() bool  { return fc&0x0400 != 0 }
func (fc FrameControl) Retry() bool     { return fc&0x0800 != 0 }
func (fc FrameControl) PwrMgt() bool    { return fc&0x1000 != 0 }
func (fc FrameControl) MoreData() bool  { return fc&0x2000 != 0 }
func (fc FrameControl) Protected() bool { return fc&0x4000 != 0 }
func (fc FrameControl) Order() bool     { return fc&0x8000 != 0 }

// WithToDS, WithFromDS and WithProtected return fc with that bit set.
func (fc FrameControl) WithToDS() FrameControl      { return fc | 0x0100 }
func (fc FrameControl) WithFromDS() FrameControl    { return fc | 0x0200 }
func (fc FrameControl) WithProtected() FrameControl { return fc | 0x4000 }

// QoS reports whether a data frame carries a QoS Control field (subtype bit 3).
func (fc FrameControl) QoS() bool { return fc.Type() == TypeData && fc.Subtype()&8 != 0 }

// Header is a parsed MAC header of a management or data frame.
type Header struct {
	FC       FrameControl
	Duration uint16
	Addr1    MAC
	Addr2    MAC
	Addr3    MAC
	SeqCtrl  uint16
	Addr4    MAC
	QoS      uint16
	Len      int // header length in bytes
}

// Seq returns the sequence number, and Frag the fragment number (IEEE 802.11-2020 9.2.4.4).
func (h *Header) Seq() uint16  { return h.SeqCtrl >> 4 }
func (h *Header) Frag() uint16 { return h.SeqCtrl & 15 }

// HasAddr4 reports whether the header has a fourth address.
func (h *Header) HasAddr4() bool { return h.FC.ToDS() && h.FC.FromDS() }

// TID returns the traffic identifier of a QoS data frame.
func (h *Header) TID() uint8 { return uint8(h.QoS & 15) }

// ParseHeader decodes the MAC header at the start of frame.
func ParseHeader(frame []byte) (Header, error) {
	var h Header
	if len(frame) < 24 {
		return h, ErrShort
	}
	h.FC = FrameControl(binary.LittleEndian.Uint16(frame))
	h.Duration = binary.LittleEndian.Uint16(frame[2:])
	copy(h.Addr1[:], frame[4:10])
	copy(h.Addr2[:], frame[10:16])
	copy(h.Addr3[:], frame[16:22])
	h.SeqCtrl = binary.LittleEndian.Uint16(frame[22:])
	h.Len = 24
	if h.FC.Type() == TypeData && h.HasAddr4() {
		if len(frame) < 30 {
			return h, ErrShort
		}
		copy(h.Addr4[:], frame[24:30])
		h.Len = 30
	}
	if h.FC.QoS() {
		if len(frame) < h.Len+2 {
			return h, ErrShort
		}
		h.QoS = binary.LittleEndian.Uint16(frame[h.Len:])
		h.Len += 2
		if h.FC.Order() {
			return h, ErrOrder
		}
	}
	return h, nil
}

// PutHeader writes h into dst and returns the header length.
// The length follows from FC: a fourth address for ToDS and FromDS data frames, and QoS Control for QoS data.
func PutHeader(dst []byte, h *Header) (int, error) {
	n := 24
	addr4 := h.FC.Type() == TypeData && h.HasAddr4()
	if addr4 {
		n += 6
	}
	if h.FC.QoS() {
		n += 2
	}
	if len(dst) < n {
		return 0, ErrBuffer
	}
	binary.LittleEndian.PutUint16(dst, uint16(h.FC))
	binary.LittleEndian.PutUint16(dst[2:], h.Duration)
	copy(dst[4:], h.Addr1[:])
	copy(dst[10:], h.Addr2[:])
	copy(dst[16:], h.Addr3[:])
	binary.LittleEndian.PutUint16(dst[22:], h.SeqCtrl)
	pos := 24
	if addr4 {
		copy(dst[24:], h.Addr4[:])
		pos = 30
	}
	if h.FC.QoS() {
		binary.LittleEndian.PutUint16(dst[pos:], h.QoS)
	}
	return n, nil
}

// SeqCtrl builds a Sequence Control field from a sequence and a fragment number.
func SeqCtrl(seq, frag uint16) uint16 { return seq<<4 | frag&15 }
