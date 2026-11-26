// Package eapol runs the WPA2-Personal supplicant: EAPOL-Key frames, the 4-way handshake and
// the group key handshake (IEEE 802.11-2020 12.7). All state comes from an injected allocator that can free.
//
// Not used over the air yet: espradio exports no raw 802.11, so the Espressif library does this work
// (espradio/DESIGN.md, Wi-Fi radio). It stays for a radio link that gives raw 802.11 frames.
package eapol

import "encoding/binary"

// EtherType is the EtherType of EAPOL frames (IEEE 802.1X-2020 11.1).
const EtherType = 0x888e

// Frame layout of an EAPOL-Key frame with a 16-byte MIC (IEEE 802.11-2020 figure 12-32).
const (
	headerLen   = 4 // version, packet type, body length
	typeKey     = 3
	descRSN     = 2
	offKeyInfo  = headerLen + 1
	offKeyLen   = headerLen + 3
	offReplay   = headerLen + 5
	offNonce    = headerLen + 13
	offIV       = headerLen + 45
	offRSC      = headerLen + 61
	offMIC      = headerLen + 77
	offDataLen  = headerLen + 93
	offData     = headerLen + 95
	micLen      = 16
	nonceLen    = 32
	rscLen      = 8
	MinFrameLen = offData
)

// Key Information bits (IEEE 802.11-2020 figure 12-33).
const (
	infoVersionMask = 0x0007
	infoVersionAES  = 2 // HMAC-SHA1-128 MIC, AES key wrap
	infoVersionAKM  = 0 // the MIC and the key wrap of the AKM, for SAE AES-128-CMAC
	infoPairwise    = 0x0008
	infoInstall     = 0x0040
	infoAck         = 0x0080
	infoMIC         = 0x0100
	infoSecure      = 0x0200
	infoError       = 0x0400
	infoRequest     = 0x0800
	infoEncrypted   = 0x1000
)

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilMemory   Error = "eapol: memory cannot be nil"
	ErrNilRandom   Error = "eapol: random source cannot be nil"
	ErrNoMemory    Error = "eapol: out of memory"
	ErrShort       Error = "eapol: frame is too short"
	ErrNotKey      Error = "eapol: frame is not an RSN EAPOL-Key frame"
	ErrVersion     Error = "eapol: wrong key descriptor version"
	ErrBuffer      Error = "eapol: reply buffer is too small"
	ErrReplay      Error = "eapol: replay counter did not increase"
	ErrMIC         Error = "eapol: MIC check failed"
	ErrState       Error = "eapol: message is not expected in this state"
	ErrNonce       Error = "eapol: ANonce changed between message 1 and 3"
	ErrKeyData     Error = "eapol: key data is malformed"
	ErrNoGTK       Error = "eapol: key data has no GTK"
	ErrRSNMismatch Error = "eapol: RSN element in message 3 differs from the beacon"
	ErrNotStarted  Error = "eapol: call Start for the association first"
	ErrClosed      Error = "eapol: supplicant is closed"
	ErrRandom      Error = "eapol: random source failed"
)

// keyFrame is a view of a received EAPOL-Key frame.
type keyFrame struct {
	raw     []byte
	version byte
	info    uint16
	replay  uint64
	nonce   []byte
	rsc     []byte // key RSC, 8 bytes
	data    []byte
}

func parseKeyFrame(b []byte, version uint16) (keyFrame, error) {
	var f keyFrame
	if len(b) < MinFrameLen {
		return f, ErrShort
	}
	if b[1] != typeKey || b[headerLen] != descRSN {
		return f, ErrNotKey
	}
	bodyLen := int(binary.BigEndian.Uint16(b[2:]))
	if headerLen+bodyLen > len(b) || bodyLen < offData-headerLen {
		return f, ErrShort
	}
	b = b[:headerLen+bodyLen]
	dataLen := int(binary.BigEndian.Uint16(b[offDataLen:]))
	if offData+dataLen > len(b) {
		return f, ErrShort
	}
	f.raw = b
	f.version = b[0]
	f.info = binary.BigEndian.Uint16(b[offKeyInfo:])
	f.replay = binary.BigEndian.Uint64(b[offReplay:])
	f.nonce = b[offNonce : offNonce+nonceLen]
	f.rsc = b[offRSC : offRSC+rscLen]
	f.data = b[offData : offData+dataLen]
	if f.info&infoVersionMask != version {
		return f, ErrVersion
	}
	return f, nil
}

// putKeyFrame writes an EAPOL-Key frame with a zero MIC field into dst and returns its length.
func putKeyFrame(dst []byte, version byte, info uint16, replay uint64, nonce, data []byte) (int, error) {
	n := offData + len(data)
	if len(dst) < n {
		return 0, ErrBuffer
	}
	clear(dst[:offData])
	dst[0], dst[1] = version, typeKey
	binary.BigEndian.PutUint16(dst[2:], uint16(n-headerLen))
	dst[headerLen] = descRSN
	binary.BigEndian.PutUint16(dst[offKeyInfo:], info)
	binary.BigEndian.PutUint64(dst[offReplay:], replay)
	copy(dst[offNonce:offNonce+nonceLen], nonce)
	binary.BigEndian.PutUint16(dst[offDataLen:], uint16(len(data)))
	copy(dst[offData:], data)
	return n, nil
}
