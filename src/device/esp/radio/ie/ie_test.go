package ie

import "testing"

func TestRSNPSKCCMP(t *testing.T) {
	// WPA2-PSK with CCMP and capabilities 0x000c.
	e := []byte{48, 20, 1, 0,
		0x00, 0x0f, 0xac, 4,
		1, 0, 0x00, 0x0f, 0xac, 4,
		1, 0, 0x00, 0x0f, 0xac, 2,
		0x0c, 0x00}
	d, code := Parse(e)
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if d.Proto != ProtoRSN || d.GroupCipher != CipherCCMP || d.PairwiseCipher != CipherCCMP ||
		d.KeyMgmt != KeyMgmtPSK || d.Capabilities != 0x0c || d.MgmtGroupCipher != CipherAES128CMAC {
		t.Errorf("got %+v", d)
	}
}

func TestRSNTransitionSAE(t *testing.T) {
	// WPA2/WPA3 transition: PSK and SAE, one PMKID, BIP-CMAC-128.
	e := []byte{48, 0, 1, 0,
		0x00, 0x0f, 0xac, 4,
		1, 0, 0x00, 0x0f, 0xac, 4,
		2, 0, 0x00, 0x0f, 0xac, 2, 0x00, 0x0f, 0xac, 8,
		0x80, 0x00,
		1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		0x00, 0x0f, 0xac, 6}
	e[1] = byte(len(e) - 2)
	d, code := Parse(e)
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if d.KeyMgmt != KeyMgmtPSK|KeyMgmtSAE || len(d.PMKID) != 16 || d.PMKID[0] != 1 {
		t.Errorf("got %+v", d)
	}
}

func TestWPATKIP(t *testing.T) {
	e := []byte{221, 22, 0x00, 0x50, 0xf2, 1, 1, 0,
		0x00, 0x50, 0xf2, 2,
		1, 0, 0x00, 0x50, 0xf2, 2,
		1, 0, 0x00, 0x50, 0xf2, 2}
	d, code := Parse(e)
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if d.Proto != ProtoWPA || d.PairwiseCipher != CipherTKIP || d.KeyMgmt != KeyMgmtPSK {
		t.Errorf("got %+v", d)
	}
	if PublicCipher(d.PairwiseCipher) != 3 {
		t.Errorf("public cipher %d", PublicCipher(d.PairwiseCipher))
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		e    []byte
		code int
	}{
		{"empty", nil, -1},
		{"bad length", []byte{48, 9, 1, 0}, -2},
		{"bad version", []byte{48, 2, 2, 0}, -2},
		{"short group", []byte{48, 4, 1, 0, 0x00, 0x0f}, -3},
		{"zero pairwise", []byte{48, 8, 1, 0, 0x00, 0x0f, 0xac, 4, 0, 0}, -4},
		{"odd byte", []byte{48, 7, 1, 0, 0x00, 0x0f, 0xac, 4, 1}, -5},
		{"bad mgmt group", []byte{48, 26, 1, 0, 0x00, 0x0f, 0xac, 4,
			1, 0, 0x00, 0x0f, 0xac, 4, 1, 0, 0x00, 0x0f, 0xac, 2, 0, 0, 0, 0,
			0x00, 0x0f, 0xac, 4}, -10},
		{"not WPA", []byte{221, 6, 0x00, 0x50, 0xf2, 2, 1, 0}, -2},
	}
	for _, tt := range tests {
		if _, code := Parse(tt.e); code != tt.code {
			t.Errorf("%s: code %d, want %d", tt.name, code, tt.code)
		}
	}
}

func TestRSNX(t *testing.T) {
	d, code := Parse([]byte{244, 1, 0x20})
	if code != 0 || d.RSNXECapa != 0x20 {
		t.Errorf("got %+v code %d", d, code)
	}
}
