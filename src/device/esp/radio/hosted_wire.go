//go:build esp32p4 || hostedwire

package radio

// The wire format of ESP-Hosted-MCU 2.x, which an ESP32-C6 runs as a Wi-Fi co-processor for an ESP32-P4: the header on every frame,
// and the control messages, which are protobuf, in common/proto/esp_hosted_rpc.proto. It's plain code, with nothing of the chip's,
// so it can be tested on a computer with the hostedwire tag.
//
// See github.com/espressif/esp-hosted-mcu at v2.12.12: common/esp_hosted_header.h, common/transport/esp_hosted_transport.h and
// host/drivers/virtual_serial_if/serial_if.c.

import "encoding/binary"

// HostedError is a string error type, so error values are constants in flash and not in RAM.
type HostedError string

func (e HostedError) Error() string { return string(e) }

// A frame's header (struct esp_payload_header): 12 bytes, little-endian.
const (
	hostedHeaderLen = 12
	hostedMaxFrame  = 1536 // ESP_RX_BUFFER_SIZE, header and all
	hostedMaxData   = hostedMaxFrame - hostedHeaderLen

	hostedIfSTA    = 1
	hostedIfSerial = 3
	hostedIfPriv   = 5

	hostedMoreFragment = 1 << 0

	hostedPacketTypeEvent = 0x33 // ESP_PACKET_TYPE_EVENT
	hostedEventInit       = 0x22 // ESP_PRIV_EVENT_INIT
)

// What the C6 says of itself when it starts (ESP_PRIV_*), and what the host says back (HOST_CAPABILITIES and on).
const (
	hostedTagCapability      = 0x11
	hostedTagChipID          = 0x12
	hostedTagFirmwareVersion = 0x17
	hostedTagSDIOMode        = 0x18

	hostedTagHostCapabilities = 0x44
	hostedTagChipIDReceived   = 0x45
	hostedTagTestRawTP        = 0x46
	hostedTagThrottleHigh     = 0x47
	hostedTagThrottleLow      = 0x48

	hostedCapChecksum = 1 << 7 // ESP_CHECKSUM_ENABLED
)

type hostedHeader struct {
	ifType, ifNum uint8
	flags         uint8
	length        uint16 // of what follows the header
	checksum      uint16
	seq           uint16
	privType      uint8 // for hostedIfPriv: ESP_PACKET_TYPE_EVENT
}

func putHostedHeader(b []byte, h hostedHeader) {
	b[0] = h.ifType&0xF | h.ifNum<<4
	b[1] = h.flags
	binary.LittleEndian.PutUint16(b[2:], h.length)
	binary.LittleEndian.PutUint16(b[4:], hostedHeaderLen)
	binary.LittleEndian.PutUint16(b[6:], h.checksum)
	binary.LittleEndian.PutUint16(b[8:], h.seq)
	b[10] = 0
	b[11] = h.privType
}

// parseHostedHeader reads a header, and false is that it isn't one: no data, too much, or the wrong size of header.
func parseHostedHeader(b []byte) (hostedHeader, bool) {
	if len(b) < hostedHeaderLen {
		return hostedHeader{}, false
	}
	h := hostedHeader{
		ifType: b[0] & 0xF, ifNum: b[0] >> 4, flags: b[1],
		length:   binary.LittleEndian.Uint16(b[2:]),
		checksum: binary.LittleEndian.Uint16(b[6:]),
		seq:      binary.LittleEndian.Uint16(b[8:]),
		privType: b[11],
	}
	if h.length == 0 || h.length > hostedMaxData || binary.LittleEndian.Uint16(b[4:]) != hostedHeaderLen {
		return h, false
	}
	return h, true
}

// hostedChecksum is compute_checksum: the bytes' sum, with the header's checksum field as 0.
func hostedChecksum(frame []byte) uint16 {
	var sum uint16
	for i, c := range frame {
		if i == 6 || i == 7 {
			continue
		}
		sum += uint16(c)
	}
	return sum
}

// The serial interface carries the control messages, each in two TLVs: an endpoint's name, then the protobuf.
const (
	hostedTLVName = 0x01
	hostedTLVData = 0x02

	hostedEndpointResponse = "RPCRsp" // what the host sends on, and the answers come on
	hostedEndpointEvent    = "RPCEvt"
)

// serialTLV wraps a protobuf for the serial interface.
func serialTLV(dst, proto []byte) []byte {
	dst = append(dst, hostedTLVName, byte(len(hostedEndpointResponse)), 0)
	dst = append(dst, hostedEndpointResponse...)
	dst = append(dst, hostedTLVData, byte(len(proto)), byte(len(proto)>>8))
	return append(dst, proto...)
}

// parseSerialTLV is the endpoint's name and the protobuf, and false if it isn't the two TLVs.
func parseSerialTLV(b []byte) (endpoint string, proto []byte, ok bool) {
	for len(b) >= 3 {
		t, n := b[0], int(binary.LittleEndian.Uint16(b[1:]))
		if len(b) < 3+n {
			return "", nil, false
		}
		v := b[3 : 3+n]
		switch t {
		case hostedTLVName:
			endpoint = string(v)
		case hostedTLVData:
			proto = v
		}
		b = b[3+n:]
	}
	return endpoint, proto, endpoint != "" && proto != nil
}

// The control messages' envelope (message Rpc): what kind, which message, a number to match an answer, and the message, in a field
// numbered as its id.
const (
	rpcRequest  = 1
	rpcResponse = 2
	rpcEvent    = 3

	rpcRespOffset = 512 - 256 // a request's answer is its id plus this

	rpcGetMACAddress    = 257
	rpcSetWifiMode      = 260
	rpcWifiSetPs        = 270
	rpcWifiGetPs        = 271
	rpcWifiInit         = 278
	rpcWifiStart        = 280
	rpcWifiConnect      = 282
	rpcWifiDisconnect   = 283
	rpcWifiSetConfig    = 284
	rpcWifiScanStart    = 286
	rpcWifiScanGetApNum = 288
	rpcWifiScanGetAps   = 289
	rpcWifiStaGetApInfo = 294

	rpcEventESPInit         = 769
	rpcEventHeartbeat       = 770
	rpcEventWifiNoArgs      = 773
	rpcEventStaScanDone     = 774
	rpcEventStaConnected    = 775
	rpcEventStaDisconnected = 776
)

// pbWriter writes protobuf. A field that's 0 or empty is left out, as proto3 does.
type pbWriter struct{ b []byte }

func (w *pbWriter) tag(field, wire int) { w.raw(uint64(field)<<3 | uint64(wire)) }

func (w *pbWriter) raw(v uint64) {
	for v >= 0x80 {
		w.b = append(w.b, byte(v)|0x80)
		v >>= 7
	}
	w.b = append(w.b, byte(v))
}

// uint writes a varint field, of any of the integer types: an int32 that's negative is its 64-bit two's complement, in 10 bytes.
func (w *pbWriter) uint(field int, v uint64) {
	if v == 0 {
		return
	}
	w.tag(field, 0)
	w.raw(v)
}

func (w *pbWriter) int(field int, v int32) { w.uint(field, uint64(int64(v))) }

func (w *pbWriter) bool(field int, v bool) {
	if v {
		w.uint(field, 1)
	}
}

func (w *pbWriter) bytes(field int, v []byte) {
	if len(v) == 0 {
		return
	}
	w.tag(field, 2)
	w.raw(uint64(len(v)))
	w.b = append(w.b, v...)
}

// message writes a field of a message, even an empty one, which is how a request with nothing in it is still the request it is.
func (w *pbWriter) message(field int, m pbWriter) {
	w.tag(field, 2)
	w.raw(uint64(len(m.b)))
	w.b = append(w.b, m.b...)
}

// pbField is a field that's been read: its number, and its value as an integer or bytes.
type pbField struct {
	num int
	v   uint64
	b   []byte
}

// pbNext reads the next field of b, and false at the end or at what isn't protobuf.
func pbNext(b []byte) (pbField, []byte, bool) {
	key, b, ok := pbVarint(b)
	if !ok {
		return pbField{}, nil, false
	}
	f := pbField{num: int(key >> 3)}
	switch key & 7 {
	case 0:
		f.v, b, ok = pbVarint(b)
	case 1:
		if len(b) < 8 {
			return f, nil, false
		}
		f.v, b = binary.LittleEndian.Uint64(b), b[8:]
	case 2:
		var n uint64
		n, b, ok = pbVarint(b)
		if !ok || n > uint64(len(b)) {
			return f, nil, false
		}
		f.b, b = b[:n], b[n:]
	case 5:
		if len(b) < 4 {
			return f, nil, false
		}
		f.v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
	default:
		return f, nil, false
	}
	return f, b, ok
}

func pbVarint(b []byte) (uint64, []byte, bool) {
	var v uint64
	for i := 0; i < 10 && i < len(b); i++ {
		v |= uint64(b[i]&0x7F) << (7 * i)
		if b[i] < 0x80 {
			return v, b[i+1:], true
		}
	}
	return 0, nil, false
}

// pbFields calls f for each field of b, and false if b isn't protobuf.
func pbFields(b []byte, f func(pbField)) bool {
	for len(b) > 0 {
		field, rest, ok := pbNext(b)
		if !ok {
			return false
		}
		f(field)
		b = rest
	}
	return true
}

// rpcEnvelope is a request: its id, its number, and its message.
func rpcEnvelope(id, uid uint32, m pbWriter) []byte {
	var w pbWriter
	w.uint(1, rpcRequest)
	w.uint(2, uint64(id))
	w.uint(3, uint64(uid))
	w.message(int(id), m)
	return w.b
}

// rpcMessage is what an envelope that's been read says.
type rpcMessage struct {
	kind, id, uid uint32
	payload       []byte
}

func parseRPC(b []byte) (rpcMessage, bool) {
	var m rpcMessage
	ok := pbFields(b, func(f pbField) {
		switch {
		case f.num == 1:
			m.kind = uint32(f.v)
		case f.num == 2:
			m.id = uint32(f.v)
		case f.num == 3:
			m.uid = uint32(f.v)
		case f.num > 3 && f.b != nil:
			m.payload = f.b
		}
	})
	return m, ok && m.id != 0
}

// rpcResult is an answer's resp field, which is esp_err_t, 0 for done: field 1 of most answers.
func rpcResult(payload []byte, field int) int32 {
	var resp int32
	pbFields(payload, func(f pbField) {
		if f.num == field {
			resp = int32(f.v)
		}
	})
	return resp
}

// hostedPrivInit is what the C6 says of itself when it starts.
type hostedPrivInit struct {
	chipID     uint8
	capability uint8
	version    uint32 // major << 16 | minor << 8 | patch
	streaming  bool
}

// parsePrivInit reads an ESP_PRIV_EVENT_INIT: its type, its length, and TLVs of a byte's tag and a byte's length.
func parsePrivInit(b []byte) (hostedPrivInit, bool) {
	var p hostedPrivInit
	if len(b) < 2 || b[0] != hostedEventInit || int(b[1]) > len(b)-2 {
		return p, false
	}
	tlvs := b[2 : 2+int(b[1])]
	for len(tlvs) >= 2 {
		t, n := tlvs[0], int(tlvs[1])
		if len(tlvs) < 2+n {
			break
		}
		v := tlvs[2 : 2+n]
		switch {
		case t == hostedTagChipID && n >= 1:
			p.chipID = v[0]
		case t == hostedTagCapability && n >= 1:
			p.capability = v[0]
		case t == hostedTagFirmwareVersion && n >= 4:
			p.version = binary.LittleEndian.Uint32(v)
		case t == hostedTagSDIOMode && n >= 1:
			p.streaming = v[0] == 1
		}
		tlvs = tlvs[2+n:]
	}
	return p, true
}

// hostConfig is what the host says back: send_slave_config, with no capabilities, no throughput test and no throttling.
func hostConfig(chipID uint8) []byte {
	tlvs := []byte{
		hostedTagHostCapabilities, 1, 0,
		hostedTagChipIDReceived, 1, chipID,
		hostedTagTestRawTP, 1, 0,
		hostedTagThrottleHigh, 1, 0,
		hostedTagThrottleLow, 1, 0,
	}
	return append([]byte{hostedEventInit, byte(len(tlvs))}, tlvs...)
}

// wifiInitConfig is WIFI_INIT_CONFIG_DEFAULT's, as the C6 takes every field from the host: ESP-IDF's defaults, with WPA3 as a feature.
func wifiInitConfig() pbWriter {
	var c pbWriter
	c.int(1, 10)          // static_rx_buf_num
	c.int(2, 32)          // dynamic_rx_buf_num
	c.int(3, 1)           // tx_buf_type: dynamic
	c.int(5, 32)          // dynamic_tx_buf_num
	c.int(8, 1)           // ampdu_rx_enable
	c.int(9, 1)           // ampdu_tx_enable
	c.int(11, 1)          // nvs_enable
	c.int(13, 6)          // rx_ba_win
	c.int(15, 752)        // beacon_max_len
	c.int(16, 32)         // mgmt_sbuf_num
	c.uint(17, 1)         // feature_caps: WPA3 SAE
	c.int(19, 7)          // espnow_max_encrypt_num
	c.int(20, 0x1F2F3F4F) // magic: WIFI_INIT_CONFIG_MAGIC
	return c
}

// staConfig is a station's wifi_config: the network, and an access point to join of it if bssid is set.
func staConfig(ssid, password string, bssid [6]byte, channel uint8, pick bool) pbWriter {
	var sta pbWriter
	sta.bytes(1, []byte(ssid))
	sta.bytes(2, []byte(password))
	if pick {
		sta.bool(4, true)
		sta.bytes(5, bssid[:])
		sta.uint(6, uint64(channel))
	}
	var pmf pbWriter
	pmf.bool(1, true) // capable
	sta.message(10, pmf)
	var cfg pbWriter
	cfg.message(2, sta)
	return cfg
}

// parseAPRecord reads a wifi_ap_record.
func parseAPRecord(b []byte) AP {
	var ap AP
	pbFields(b, func(f pbField) {
		switch f.num {
		case 1:
			copy(ap.BSSID[:], f.b)
		case 2:
			ap.SSID = cString(f.b)
		case 3:
			ap.Channel = uint8(f.v)
		case 5:
			ap.RSSI = int8(int32(f.v))
		case 6:
			ap.Auth = AuthMode(int32(f.v))
		}
	})
	return ap
}

// cString is b up to its first zero: an SSID comes in a buffer of 33.
func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
