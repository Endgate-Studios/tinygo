package ieee80211

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

var (
	sta   = MAC{0x02, 0, 0, 0, 0, 0x01}
	ap    = MAC{0x02, 0, 0, 0, 0, 0xaa}
	other = MAC{0x02, 0, 0, 0, 0, 0x33}
)

func TestParseHeaderCCMPVector(t *testing.T) {
	// MAC header of the CCMP test MPDU in IEEE 802.11-2020 annex J.6.4.
	h, err := ParseHeader(unhex(t, "08 48 c3 2c 0f d2 e1 28 a5 7c 50 30 f1 84 44 08 ab ae a5 b8 fc ba 80 33"))
	if err != nil {
		t.Fatal(err)
	}
	if h.FC.Type() != TypeData || h.FC.Subtype() != SubtypeData || !h.FC.Retry() || !h.FC.Protected() || h.FC.ToDS() || h.FC.FromDS() {
		t.Errorf("frame control %#04x decoded wrong", uint16(h.FC))
	}
	if h.Duration != 11459 || h.Seq() != 824 || h.Frag() != 0 || h.Len != 24 {
		t.Errorf("duration %d, seq %d, frag %d, len %d; want 11459, 824, 0, 24", h.Duration, h.Seq(), h.Frag(), h.Len)
	}
	if h.Addr1 != (MAC{0x0f, 0xd2, 0xe1, 0x28, 0xa5, 0x7c}) || h.Addr2 != (MAC{0x50, 0x30, 0xf1, 0x84, 0x44, 0x08}) {
		t.Errorf("addresses = %x %x", h.Addr1, h.Addr2)
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		h       Header
		wantLen int
	}{
		{name: "management", h: Header{FC: NewFrameControl(TypeMgmt, SubtypeBeacon), Addr1: Broadcast, Addr2: ap, Addr3: ap, SeqCtrl: SeqCtrl(100, 0)}, wantLen: 24},
		{name: "QoS data to DS", h: Header{FC: NewFrameControl(TypeData, SubtypeQoSData).WithToDS(), Addr1: ap, Addr2: sta, Addr3: other, SeqCtrl: SeqCtrl(7, 1), QoS: 5}, wantLen: 26},
		{name: "four addresses QoS", h: Header{FC: NewFrameControl(TypeData, SubtypeQoSData).WithToDS().WithFromDS(), Addr1: ap, Addr2: sta, Addr3: other, Addr4: other, QoS: 3}, wantLen: 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 40)
			n, err := PutHeader(buf, &tt.h)
			if err != nil || n != tt.wantLen {
				t.Fatalf("PutHeader = %d, %v, want %d", n, err, tt.wantLen)
			}
			got, err := ParseHeader(buf[:n])
			tt.h.Len = tt.wantLen
			if err != nil || got != tt.h {
				t.Errorf("ParseHeader = %+v, %v\nwant %+v", got, err, tt.h)
			}
			if _, err := PutHeader(buf[:n-1], &tt.h); err != ErrBuffer {
				t.Errorf("short buffer error = %v, want ErrBuffer", err)
			}
		})
	}
	if _, err := ParseHeader(make([]byte, 23)); err != ErrShort {
		t.Errorf("short header error = %v, want ErrShort", err)
	}
}

func TestProbeRequest(t *testing.T) {
	rates := []byte{0x82, 0x84, 0x8b, 0x96, 0x0c, 0x12, 0x18, 0x24, 0x30, 0x48, 0x60, 0x6c}
	buf := make([]byte, 128)
	n, err := PutProbeRequest(buf, sta, 9, []byte("home"), rates)
	if err != nil {
		t.Fatal(err)
	}
	want := unhex(t, "40 00 00 00 ffffffffffff 020000000001 ffffffffffff 9000"+
		" 00 04 686f6d65"+
		" 01 08 82848b960c121824"+
		" 32 04 3048606c")
	if !bytes.Equal(buf[:n], want) {
		t.Fatalf("probe request =\n%x\nwant\n%x", buf[:n], want)
	}
	h, body, err := Mgmt(buf[:n])
	if err != nil || h.FC.Subtype() != SubtypeProbeReq || string(Elements(body).SSID()) != "home" {
		t.Errorf("parse back: subtype %d, SSID %q, %v", h.FC.Subtype(), Elements(body).SSID(), err)
	}
	if _, err := PutProbeRequest(buf, sta, 0, make([]byte, 33), rates); err != ErrSSID {
		t.Errorf("33-byte SSID error = %v, want ErrSSID", err)
	}
}

// wpa2PSK is the data of the usual WPA2-Personal RSN element: CCMP group and pairwise, PSK AKM, capabilities 0x000c.
const wpa2PSK = "0100 000fac04 0100 000fac04 0100 000fac02 0c00"

func TestRSN(t *testing.T) {
	r, err := ParseRSN(unhex(t, wpa2PSK))
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != 1 || r.Group != CipherCCMP || !r.HasPairwise(CipherCCMP) || !r.HasAKM(AKMPSK) || r.HasAKM(AKMSAE) || r.Capabilities != 0x000c {
		t.Errorf("ParseRSN = %+v", r)
	}
	buf := make([]byte, 32)
	n, err := AppendRSN(buf, 0, CipherCCMP, CipherCCMP, AKMPSK, 0x000c)
	if err != nil || !bytes.Equal(buf[:n], append([]byte{EIDRSN, 20}, unhex(t, wpa2PSK)...)) {
		t.Errorf("AppendRSN = %x, %v", buf[:n], err)
	}

	tests := []struct {
		name    string
		data    string
		wantErr error
	}{
		{name: "version only, defaults", data: "0100"},
		{name: "empty", data: "", wantErr: ErrRSN},
		{name: "count past the end", data: "0100 000fac04 0200 000fac04", wantErr: ErrRSN},
		{name: "too many suites", data: "0100 000fac04 0500", wantErr: ErrTooMany},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRSN(unhex(t, tt.data))
			if err != tt.wantErr {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (!r.HasPairwise(CipherCCMP) || !r.HasAKM(AKMPSK)) {
				t.Errorf("defaults missing: %+v", r)
			}
		})
	}
}

func TestBeacon(t *testing.T) {
	body := unhex(t, "0102030405060708 6400 3104"+
		" 00 04 686f6d65"+
		" 01 04 82848b96"+
		" 03 01 06"+
		" 30 14 "+wpa2PSK)
	b, err := ParseBeacon(body)
	if err != nil {
		t.Fatal(err)
	}
	if b.Timestamp != 0x0807060504030201 || b.Interval != 100 || b.Capability&CapPrivacy == 0 {
		t.Errorf("fixed fields = %+v", b)
	}
	if string(b.Elements.SSID()) != "home" || b.Elements.Channel() != 6 {
		t.Errorf("SSID %q, channel %d", b.Elements.SSID(), b.Elements.Channel())
	}
	el, ok := b.Elements.Find(EIDRSN)
	if !ok {
		t.Fatal("no RSN element")
	}
	if r, err := ParseRSN(el.Data); err != nil || !r.HasAKM(AKMPSK) {
		t.Errorf("RSN = %+v, %v", r, err)
	}
	if _, err := ParseBeacon(append(body, 0x30, 0x10, 0x01)); err != ErrIE {
		t.Errorf("truncated element error = %v, want ErrIE", err)
	}
}

func TestAuthAndAssoc(t *testing.T) {
	buf := make([]byte, 128)
	n, err := PutAuth(buf, ap, sta, 1)
	if err != nil {
		t.Fatal(err)
	}
	h, body, _ := Mgmt(buf[:n])
	a, err := ParseAuth(body)
	if err != nil || h.FC.Subtype() != SubtypeAuth || a.Algorithm != AuthOpen || a.Seq != 1 || a.Status != StatusSuccess || h.Addr1 != ap {
		t.Errorf("auth = %+v, %v", a, err)
	}

	var rsn [22]byte
	rsnLen, _ := AppendRSN(rsn[:], 0, CipherCCMP, CipherCCMP, AKMPSK, 0)
	n, err = PutAssocRequest(buf, ap, sta, 2, CapESS|CapPrivacy, 10, []byte("home"), []byte{0x82, 0x84}, rsn[2:rsnLen])
	if err != nil {
		t.Fatal(err)
	}
	h, body, _ = Mgmt(buf[:n])
	if h.FC.Subtype() != SubtypeAssocReq || len(body) < 4 {
		t.Fatalf("assoc request subtype %d", h.FC.Subtype())
	}
	els := Elements(body[4:])
	if string(els.SSID()) != "home" {
		t.Errorf("assoc SSID = %q", els.SSID())
	}
	if el, ok := els.Find(EIDRSN); !ok || !bytes.Equal(el.Data, rsn[2:rsnLen]) {
		t.Error("assoc request lost the RSN element")
	}

	resp, err := ParseAssocResp(unhex(t, "1104 0000 01c0 010482848b96"))
	if err != nil || resp.Status != StatusSuccess || resp.AID != 1 {
		t.Errorf("assoc response = %+v, %v", resp, err)
	}

	n, err = PutDeauth(buf, ap, sta, ap, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	_, body, _ = Mgmt(buf[:n])
	if reason, err := ParseReason(body); err != nil || reason != 3 {
		t.Errorf("deauth reason = %d, %v", reason, err)
	}
}

func TestDataPath(t *testing.T) {
	eth := append(append(append([]byte{}, other[:]...), sta[:]...), 0x08, 0x00)
	eth = append(eth, []byte("ip packet")...)
	buf := make([]byte, 128)
	n, err := ToDSData(buf, eth, ap, 5, true, 6)
	if err != nil {
		t.Fatal(err)
	}
	h, err := ParseHeader(buf[:n])
	if err != nil || !h.FC.ToDS() || h.FC.FromDS() || h.Addr1 != ap || h.Addr2 != sta || h.Addr3 != other || h.TID() != 6 {
		t.Fatalf("to-DS header = %+v, %v", h, err)
	}
	if et, err := EtherType(buf[h.Len:n]); err != nil || et != 0x0800 {
		t.Errorf("EtherType = %#x, %v", et, err)
	}

	// The access point sends it back from the DS: Addr1 = destination, Addr2 = BSSID, Addr3 = source.
	from := Header{FC: NewFrameControl(TypeData, SubtypeData).WithFromDS(), Addr1: other, Addr2: ap, Addr3: sta}
	fn, _ := PutHeader(buf, &from)
	fn += copy(buf[fn:], []byte{0xaa, 0xaa, 0x03, 0, 0, 0, 0x08, 0x00})
	fn += copy(buf[fn:], "ip packet")
	out := make([]byte, 64)
	m, err := FromDSData(out, buf[:fn])
	if err != nil || !bytes.Equal(out[:m], eth) {
		t.Errorf("FromDSData = %x, %v\nwant %x", out[:m], err, eth)
	}
	buf[24] = 0x42
	if _, err := FromDSData(out, buf[:fn]); err != ErrSNAP {
		t.Errorf("no SNAP error = %v, want ErrSNAP", err)
	}
}
