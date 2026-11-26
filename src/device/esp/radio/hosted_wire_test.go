//go:build hostedwire && !esp32p4

package radio

// The ESP32-C6's wire format, on a computer: go test -tags hostedwire, with this package's directory as a module of its own.

import (
	"bytes"
	"testing"
)

// What radio_esp32p4.go has, that hosted_wire.go uses.
type AuthMode int32

type AP struct {
	SSID    string
	BSSID   [6]byte
	Channel uint8
	RSSI    int8
	Auth    AuthMode
}

func TestHeaderComesBack(t *testing.T) {
	want := hostedHeader{ifType: hostedIfSerial, ifNum: 2, flags: hostedMoreFragment, length: 300, checksum: 0xBEEF, seq: 7, privType: 0x33}
	b := make([]byte, hostedHeaderLen)
	putHostedHeader(b, want)
	if b[0] != 0x23 || b[4] != hostedHeaderLen {
		t.Errorf("% x", b)
	}
	got, ok := parseHostedHeader(b)
	if !ok || got != want {
		t.Errorf("%+v, %v", got, ok)
	}
}

func TestHeaderThatIsnt(t *testing.T) {
	b := make([]byte, hostedHeaderLen)
	if _, ok := parseHostedHeader(b); ok {
		t.Error("padding was a header")
	}
	putHostedHeader(b, hostedHeader{ifType: 1, length: hostedMaxData + 1})
	if _, ok := parseHostedHeader(b); ok {
		t.Error("a frame too long for the C6 was a header")
	}
}

// compute_checksum sums the bytes with the checksum's own as 0.
func TestChecksum(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5, 6, 0xAA, 0xBB, 9, 10}
	if got, want := hostedChecksum(b), uint16(1+2+3+4+5+6+9+10); got != want {
		t.Errorf("%d, not %d", got, want)
	}
}

func TestSerialTLV(t *testing.T) {
	b := serialTLV(nil, []byte{8, 1})
	want := []byte{1, 6, 0, 'R', 'P', 'C', 'R', 's', 'p', 2, 2, 0, 8, 1}
	if !bytes.Equal(b, want) {
		t.Errorf("% x", b)
	}
	endpoint, proto, ok := parseSerialTLV(b)
	if !ok || endpoint != hostedEndpointResponse || !bytes.Equal(proto, []byte{8, 1}) {
		t.Errorf("%q % x %v", endpoint, proto, ok)
	}
	if _, _, ok := parseSerialTLV(b[:10]); ok {
		t.Error("cut short and read")
	}
}

// A request's envelope: proto3 leaves 0 out, and a message with nothing in it is still there, at its id.
func TestEnvelope(t *testing.T) {
	var m pbWriter
	m.int(1, 1)
	b := rpcEnvelope(rpcSetWifiMode, 5, m)
	want := []byte{0x08, 1, 0x10, 0x84, 0x02, 0x18, 5, 0xA2, 0x10, 2, 0x08, 1}
	if !bytes.Equal(b, want) {
		t.Errorf("% x\n% x", b, want)
	}
	got, ok := parseRPC(b)
	if !ok || got.kind != rpcRequest || got.id != rpcSetWifiMode || got.uid != 5 || !bytes.Equal(got.payload, []byte{8, 1}) {
		t.Errorf("%+v", got)
	}
	empty := rpcEnvelope(rpcWifiStart, 1, pbWriter{})
	if !bytes.HasSuffix(empty, []byte{0xC2, 0x11, 0}) {
		t.Errorf("% x", empty)
	}
}

// An answer's resp is an int32, which is negative in 10 bytes.
func TestResult(t *testing.T) {
	var w pbWriter
	w.int(1, -1)
	if len(w.b) != 11 {
		t.Errorf("% x", w.b)
	}
	if got := rpcResult(w.b, 1); got != -1 {
		t.Errorf("%d", got)
	}
	w = pbWriter{}
	w.int(1, 0x3001)
	if got := rpcResult(w.b, 1); got != 0x3001 {
		t.Errorf("%d", got)
	}
	if got := rpcResult(nil, 1); got != 0 {
		t.Errorf("nothing is %d", got)
	}
}

func TestPrivInit(t *testing.T) {
	tlvs := []byte{
		hostedTagCapability, 1, 0x81,
		hostedTagChipID, 1, 0x0D,
		0x13, 1, 0, // a test's, which isn't kept
		hostedTagFirmwareVersion, 4, 12, 12, 2, 0,
		hostedTagSDIOMode, 1, 1,
	}
	b := append([]byte{hostedEventInit, byte(len(tlvs))}, tlvs...)
	p, ok := parsePrivInit(b)
	if !ok || p.chipID != 0x0D || p.capability != 0x81 || p.version != 2<<16|12<<8|12 || !p.streaming {
		t.Errorf("%+v %v", p, ok)
	}
	if _, ok := parsePrivInit([]byte{hostedEventInit, 40, 1}); ok {
		t.Error("cut short and read")
	}
}

func TestHostConfig(t *testing.T) {
	b := hostConfig(0x0D)
	if b[0] != hostedEventInit || int(b[1]) != len(b)-2 || b[7] != 0x0D {
		t.Errorf("% x", b)
	}
}

// What's sent to join a network, read back as the C6 reads it.
func TestStaConfig(t *testing.T) {
	cfg := staConfig("home", "made-up-password", [6]byte{1, 2, 3, 4, 5, 6}, 11, true)
	var sta []byte
	pbFields(cfg.b, func(f pbField) {
		if f.num == 2 {
			sta = f.b
		}
	})
	got := map[int]pbField{}
	if !pbFields(sta, func(f pbField) { got[f.num] = f }) {
		t.Fatal("not protobuf")
	}
	if string(got[1].b) != "home" || string(got[2].b) != "made-up-password" || got[4].v != 1 || !bytes.Equal(got[5].b, []byte{1, 2, 3, 4, 5, 6}) || got[6].v != 11 {
		t.Errorf("%+v", got)
	}
	if _, ok := got[10]; !ok {
		t.Error("no PMF")
	}
	any := staConfig("home", "", [6]byte{}, 0, false)
	pbFields(any.b, func(f pbField) {
		pbFields(f.b, func(g pbField) {
			if g.num == 4 || g.num == 5 || g.num == 2 {
				t.Errorf("field %d with no access point picked and no password", g.num)
			}
		})
	})
}

func TestAPRecord(t *testing.T) {
	var w pbWriter
	w.bytes(1, []byte{9, 8, 7, 6, 5, 4})
	w.bytes(2, append([]byte("cafe"), make([]byte, 29)...))
	w.uint(3, 6)
	w.int(5, -67)
	w.int(6, 3)
	ap := parseAPRecord(w.b)
	if ap.SSID != "cafe" || ap.BSSID != [6]byte{9, 8, 7, 6, 5, 4} || ap.Channel != 6 || ap.RSSI != -67 || ap.Auth != 3 {
		t.Errorf("%+v", ap)
	}
}

func TestWifiInitConfigHasMagic(t *testing.T) {
	c := wifiInitConfig()
	var magic uint64
	pbFields(c.b, func(f pbField) {
		if f.num == 20 {
			magic = f.v
		}
	})
	if magic != 0x1F2F3F4F {
		t.Errorf("%#x", magic)
	}
}

func TestNotProtobuf(t *testing.T) {
	if pbFields([]byte{0x0A, 5, 1}, func(pbField) {}) {
		t.Error("a field past the end was read")
	}
	if _, ok := parseRPC([]byte{0xFF}); ok {
		t.Error("read")
	}
}
