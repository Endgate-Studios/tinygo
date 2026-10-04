package hci

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var peer = Addr{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}

func TestCommands(t *testing.T) {
	buf := make([]byte, 64)
	tests := []struct {
		name string
		put  func() (int, error)
		want string
	}{
		{name: "reset", put: func() (int, error) { return PutReset(buf) }, want: "01 030c 00"},
		{name: "LE event mask", put: func() (int, error) { return PutSetEventMask(buf, true, 0x1f) }, want: "01 0120 08 1f00000000000000"},
		{name: "advertising enable", put: func() (int, error) { return PutLESetAdvEnable(buf, true) }, want: "01 0a20 01 01"},
		{name: "scan enable", put: func() (int, error) { return PutLESetScanEnable(buf, true, true) }, want: "01 0c20 02 0101"},
		{name: "scan parameters", put: func() (int, error) { return PutLESetScanParams(buf, true, 0x0010, 0x0010, AddrPublic, 0) }, want: "01 0b20 07 01 1000 1000 00 00"},
		{name: "disconnect", put: func() (int, error) { return PutDisconnect(buf, 0x0040, 0x13) }, want: "01 0604 03 4000 13"},
		{
			name: "advertising parameters",
			put: func() (int, error) {
				return PutLESetAdvParams(buf, &AdvParams{IntervalMin: 0x00a0, IntervalMax: 0x00a0, Type: AdvInd, ChannelMap: ChannelsAll})
			},
			want: "01 0620 0f a000 a000 00 00 00 000000000000 07 00",
		},
		{
			name: "advertising data padded to 31 bytes",
			put:  func() (int, error) { return PutLESetAdvData(buf, false, unhex(t, "020106")) },
			want: "01 0820 20 03 020106" + strings.Repeat("00", 28),
		},
		{
			name: "create connection",
			put: func() (int, error) {
				return PutLECreateConnection(buf, &ConnParams{ScanInterval: 0x60, ScanWindow: 0x30, PeerAddr: peer, IntervalMin: 0x18, IntervalMax: 0x28, SupervisionTimeout: 0x1f4})
			},
			want: "01 0d20 19 6000 3000 00 00 112233445566 00 1800 2800 0000 f401 0000 0000",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := tt.put()
			if err != nil {
				t.Fatal(err)
			}
			if want := unhex(t, tt.want); !bytes.Equal(buf[:n], want) {
				t.Errorf("packet = %x\nwant     %x", buf[:n], want)
			}
			c, err := ParseCommand(buf[:n])
			if err != nil || n != 4+len(c.Params) {
				t.Errorf("ParseCommand = %+v, %v", c, err)
			}
		})
	}
	if _, err := PutLESetAdvData(buf, false, make([]byte, 32)); err != ErrParams {
		t.Errorf("32 bytes of advertising data error = %v, want ErrParams", err)
	}
	if _, err := PutReset(buf[:3]); err != ErrBuffer {
		t.Errorf("small buffer error = %v, want ErrBuffer", err)
	}
	if op := NewOpcode(0x08, 0x000a); op != OpLESetAdvEnable || op.OGF() != 0x08 || op.OCF() != 0x0a {
		t.Errorf("NewOpcode = %#x", uint16(op))
	}
}

func TestCommandCompleteAndStatus(t *testing.T) {
	e, err := ParseEvent(unhex(t, "04 0e 04 01 030c 00"))
	if err != nil {
		t.Fatal(err)
	}
	cc, ok := e.CommandComplete()
	if !ok || cc.NumPackets != 1 || cc.Op != OpReset || !bytes.Equal(cc.Return, []byte{0}) {
		t.Errorf("CommandComplete = %+v, %v", cc, ok)
	}
	if _, ok := e.CommandStatus(); ok {
		t.Error("a Command Complete event decoded as Command Status")
	}

	e, _ = ParseEvent(unhex(t, "04 0f 04 00 01 0d20"))
	cs, ok := e.CommandStatus()
	if !ok || cs.Status != 0 || cs.NumPackets != 1 || cs.Op != OpLECreateConnection {
		t.Errorf("CommandStatus = %+v, %v", cs, ok)
	}

	for _, bad := range []string{"04 0e 05 01 030c", "02 0e 00", "04"} {
		if _, err := ParseEvent(unhex(t, bad)); err == nil {
			t.Errorf("ParseEvent(%s) accepted a bad packet", bad)
		}
	}
}

func TestLEConnectionComplete(t *testing.T) {
	e, err := ParseEvent(unhex(t, "04 3e 13 01 00 4000 01 00 112233445566 1800 0000 c800 00"))
	if err != nil {
		t.Fatal(err)
	}
	sub, p, ok := e.LEMeta()
	if !ok || sub != SubConnectionComplete {
		t.Fatalf("LEMeta = %d, %v", sub, ok)
	}
	c, err := ParseConnection(p)
	want := Connection{Handle: 0x40, Role: 1, PeerAddr: peer, Interval: 0x18, Timeout: 0xc8}
	if err != nil || c != want {
		t.Errorf("ParseConnection = %+v, %v\nwant %+v", c, err, want)
	}
	if _, err := ParseConnection(p[:10]); err != ErrMalformed {
		t.Errorf("short parameters error = %v", err)
	}
}

func TestAdvertisingReports(t *testing.T) {
	// Two reports: arrays of event types, address types, addresses, data lengths, data and RSSI (7.7.65.2).
	p := unhex(t, "02 00 04 00 01 112233445566 aabbccddeeff 03 02 020106 0908 c4 d0")
	r, err := ParseAdvReports(p)
	if err != nil {
		t.Fatal(err)
	}
	first, ok := r.Next()
	if !ok || first.EventType != AdvInd || first.AddrType != AddrPublic || first.Addr != peer || !bytes.Equal(first.Data, unhex(t, "020106")) || first.RSSI != -60 {
		t.Errorf("first report = %+v", first)
	}
	second, ok := r.Next()
	if !ok || second.EventType != 0x04 || second.AddrType != AddrRandom || !bytes.Equal(second.Data, unhex(t, "0908")) || second.RSSI != -48 {
		t.Errorf("second report = %+v", second)
	}
	if _, ok := r.Next(); ok {
		t.Error("iterator returned a third report")
	}
	if _, err := ParseAdvReports(p[:20]); err != ErrMalformed {
		t.Errorf("truncated reports error = %v", err)
	}
}

func TestDisconnectionAndCompletedPackets(t *testing.T) {
	e, _ := ParseEvent(unhex(t, "04 05 04 00 4000 13"))
	d, ok := e.Disconnection()
	if !ok || d.Handle != 0x40 || d.Reason != 0x13 {
		t.Errorf("Disconnection = %+v, %v", d, ok)
	}

	e, _ = ParseEvent(unhex(t, "04 13 09 02 4000 4100 0200 0100"))
	it, ok := e.CompletedPackets()
	if !ok {
		t.Fatal("CompletedPackets failed")
	}
	h, c, _ := it.Next()
	h2, c2, _ := it.Next()
	if h != 0x40 || c != 2 || h2 != 0x41 || c2 != 1 {
		t.Errorf("completed packets = (%#x, %d), (%#x, %d)", h, c, h2, c2)
	}

	f := NewFlow(3)
	for i := 0; i < 3; i++ {
		f.TookACL()
	}
	if f.CanSendACL() {
		t.Error("ACL credit not used up")
	}
	f.Update(e)
	if !f.CanSendACL() {
		t.Error("completed packets did not return ACL credits")
	}
	f.TookCommand()
	if f.CanSendCommand() {
		t.Error("command credit not used up")
	}
	cc, _ := ParseEvent(unhex(t, "04 0e 04 01 030c 00"))
	f.Update(cc)
	if !f.CanSendCommand() {
		t.Error("Command Complete did not return a command credit")
	}
}

func TestACL(t *testing.T) {
	buf := make([]byte, 32)
	n, err := PutACL(buf, 0x0040, PBFirstNonFlushable, []byte{0x03, 0x00, 0x04, 0x00, 0x0a, 0x01, 0x00})
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "02 4000 0700 0300 0400 0a0100"); !bytes.Equal(buf[:n], want) {
		t.Errorf("ACL = %x, want %x", buf[:n], want)
	}
	in, err := ParseACL(unhex(t, "02 4020 0300 aabbcc"))
	if err != nil || in.Handle != 0x40 || in.PB != PBFirstFlushable || !bytes.Equal(in.Data, unhex(t, "aabbcc")) {
		t.Errorf("ParseACL = %+v, %v", in, err)
	}
	if _, err := ParseACL(unhex(t, "02 4020 0400 aabbcc")); err != ErrShort {
		t.Errorf("truncated ACL error = %v, want ErrShort", err)
	}
}

func TestLTKRequestAndReply(t *testing.T) {
	e, _ := ParseEvent(unhex(t, "04 3e 0d 05 4000 0102030405060708 3412"))
	_, p, _ := e.LEMeta()
	r, err := ParseLTKRequest(p)
	if err != nil || r.Handle != 0x40 || r.Rand != 0x0807060504030201 || r.EDIV != 0x1234 {
		t.Errorf("ParseLTKRequest = %+v, %v", r, err)
	}
	buf := make([]byte, 32)
	var ltk [16]byte
	for i := range ltk {
		ltk[i] = byte(i)
	}
	n, err := PutLELongTermKeyReply(buf, 0x40, &ltk)
	if err != nil || !bytes.Equal(buf[:n], unhex(t, "01 1a20 12 4000 000102030405060708090a0b0c0d0e0f")) {
		t.Errorf("LTK reply = %x, %v", buf[:n], err)
	}
}

func TestEncryptionEvents(t *testing.T) {
	tests := []struct {
		name string
		pkt  string
		want EncryptionChange
		ok   bool
	}{
		{name: "change on", pkt: "04 08 04 00 4000 01", want: EncryptionChange{Handle: 0x40, Enabled: true}, ok: true},
		{name: "change off with status", pkt: "04 08 04 06 4000 00", want: EncryptionChange{Status: 6, Handle: 0x40}, ok: true},
		{name: "key refresh", pkt: "04 30 03 00 4100", want: EncryptionChange{Handle: 0x41, Enabled: true}, ok: true},
		{name: "short", pkt: "04 08 03 00 4000", ok: false},
		{name: "other event", pkt: "04 05 04 00 4000 13", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := ParseEvent(unhex(t, tt.pkt))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := e.EncryptionChange()
			if ok != tt.ok || got != tt.want {
				t.Errorf("EncryptionChange = %+v, %v, want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
	buf := make([]byte, 8)
	n, err := PutLELongTermKeyNegReply(buf, 0x40)
	if err != nil || !bytes.Equal(buf[:n], unhex(t, "01 1b20 02 4000")) {
		t.Errorf("negative reply = %x, %v", buf[:n], err)
	}
}

func TestLEEnableEncryption(t *testing.T) {
	var ltk [16]byte
	for i := range ltk {
		ltk[i] = byte(0xf0 + i)
	}
	buf := make([]byte, 40)
	n, err := PutLEEnableEncryption(buf, 0x41, 0, 0, &ltk)
	want := unhex(t, "01 1920 1c 4100 0000000000000000 0000 f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
	if err != nil || !bytes.Equal(buf[:n], want) {
		t.Errorf("LE Enable Encryption = % x, %v", buf[:n], err)
	}
}

func TestConnectionUpdate(t *testing.T) {
	buf := make([]byte, 32)
	n, err := PutLEConnectionUpdate(buf, &ConnUpdate{Handle: 0x41, IntervalMin: 6, IntervalMax: 12, Latency: 1, SupervisionTimeout: 100})
	if err != nil || !bytes.Equal(buf[:n], unhex(t, "01 1320 0e 4100 0600 0c00 0100 6400 0000 0000")) {
		t.Errorf("LE Connection Update = % x, %v", buf[:n], err)
	}
	u, err := ParseConnectionUpdate(unhex(t, "00 4100 0c00 0100 6400"))
	if err != nil || u != (ConnectionUpdate{Handle: 0x41, Interval: 12, Latency: 1, Timeout: 100}) {
		t.Errorf("ParseConnectionUpdate = %+v, %v", u, err)
	}
	if _, err := ParseConnectionUpdate(unhex(t, "00 4100")); err != ErrMalformed {
		t.Errorf("short update: %v", err)
	}
}
