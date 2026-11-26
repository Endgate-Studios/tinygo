package ieee80211

import "encoding/binary"

// snap is the LLC/SNAP header for an EtherType payload (IEEE 802.11-2020 annex Q, RFC 1042).
var snap = [6]byte{0xaa, 0xaa, 0x03, 0x00, 0x00, 0x00}

// SNAPLen is the length of the LLC/SNAP header plus the EtherType.
const SNAPLen = 8

// Ethernet header length: destination, source and EtherType.
const EthLen = 14

// ToDSData writes a data frame from a station to its access point that carries the Ethernet
// frame eth. It returns the frame length. Addr1 is the BSSID, Addr2 the source, Addr3 the destination
// (IEEE 802.11-2020 table 9-30). qos adds a QoS Control field with tid.
func ToDSData(dst []byte, eth []byte, bssid MAC, seq uint16, qos bool, tid uint8) (int, error) {
	if len(eth) < EthLen {
		return 0, ErrShort
	}
	var h Header
	subtype := uint8(SubtypeData)
	if qos {
		subtype = SubtypeQoSData
		h.QoS = uint16(tid & 15)
	}
	h.FC = NewFrameControl(TypeData, subtype).WithToDS()
	h.Addr1 = bssid
	copy(h.Addr2[:], eth[6:12])
	copy(h.Addr3[:], eth[0:6])
	h.SeqCtrl = SeqCtrl(seq, 0)
	pos, err := PutHeader(dst, &h)
	if err != nil {
		return 0, err
	}
	body := eth[EthLen:]
	if len(dst) < pos+SNAPLen+len(body) {
		return 0, ErrBuffer
	}
	copy(dst[pos:], snap[:])
	copy(dst[pos+6:], eth[12:14])
	copy(dst[pos+SNAPLen:], body)
	return pos + SNAPLen + len(body), nil
}

// FromDSData writes the Ethernet frame carried by a data frame from an access point into dst,
// and returns its length. The destination is Addr1 and the source is Addr3 (IEEE 802.11-2020 table 9-30).
// frame must be decrypted and must not hold a CCMP header.
func FromDSData(dst []byte, frame []byte) (int, error) {
	h, err := ParseHeader(frame)
	if err != nil {
		return 0, err
	}
	if h.FC.Type() != TypeData {
		return 0, ErrNotData
	}
	body := frame[h.Len:]
	if len(body) < SNAPLen || [6]byte(body[:6]) != snap {
		return 0, ErrSNAP
	}
	n := EthLen + len(body) - SNAPLen
	if len(dst) < n {
		return 0, ErrBuffer
	}
	src := h.Addr3
	if !h.FC.FromDS() {
		src = h.Addr2
	}
	copy(dst[0:], h.Addr1[:])
	copy(dst[6:], src[:])
	copy(dst[12:], body[6:8])
	copy(dst[EthLen:], body[SNAPLen:])
	return n, nil
}

// EtherType returns the EtherType of a data frame body that starts with LLC/SNAP.
func EtherType(body []byte) (uint16, error) {
	if len(body) < SNAPLen || [6]byte(body[:6]) != snap {
		return 0, ErrSNAP
	}
	return binary.BigEndian.Uint16(body[6:]), nil
}
