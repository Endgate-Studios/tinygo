// Package ie parses the WPA and RSN elements of beacons, as wpa_parse_wpa_ie
// of the ESP-IDF v5.3.2 supplicant (wpa_supplicant/src/common/wpa_common.c).
// It has the suites of the default ESP32-S3 configuration: PMF, WPA3 SAE and
// OWE, but not 802.11r, GCMP or Suite B.
package ie

// Element IDs of wpa_supplicant/src/common/ieee802_11_defs.h.
const (
	eidRSN            = 48
	eidWAPI           = 68
	eidVendorSpecific = 221
	eidRSNX           = 244
)

// Protocols of wpa_supplicant/src/common/defs.h.
const (
	ProtoWPA = 1 << 0
	ProtoRSN = 1 << 1
)

// Ciphers of wpa_supplicant/src/common/defs.h.
const (
	CipherNone       = 1 << 0
	CipherTKIP       = 1 << 1
	CipherCCMP       = 1 << 3
	CipherAES128CMAC = 1 << 5
	CipherWEP40      = 1 << 7
	CipherWEP104     = 1 << 8
)

// Key management of wpa_supplicant/src/common/defs.h.
const (
	KeyMgmtIEEE8021X       = 1 << 0
	KeyMgmtPSK             = 1 << 1
	KeyMgmtWPANone         = 1 << 4
	KeyMgmtIEEE8021XSHA256 = 1 << 7
	KeyMgmtPSKSHA256       = 1 << 8
	KeyMgmtSAE             = 1 << 10
	KeyMgmtOWE             = 1 << 22
	KeyMgmtSAEExtKey       = 1 << 26
)

// Data is struct wpa_ie_data. PMKID is a part of the parsed element.
type Data struct {
	Proto           int
	PairwiseCipher  int
	GroupCipher     int
	KeyMgmt         int
	Capabilities    int
	PMKID           []byte // NumPMKID entries of 16 bytes
	MgmtGroupCipher int
	RSNXECapa       uint8
}

const (
	selectorLen = 4
	pmkidLen    = 16
)

var (
	ouiRSN = [3]byte{0x00, 0x0f, 0xac}
	ouiWPA = [3]byte{0x00, 0x50, 0xf2}
)

// Parse parses an element with its ID and length bytes. It gives the result
// code of wpa_parse_wpa_ie: 0 on success and a negative value on error.
func Parse(ie []byte) (Data, int) {
	switch {
	case len(ie) >= 1 && ie[0] == eidRSN:
		return parseRSN(ie)
	case len(ie) >= 1 && ie[0] == eidRSNX:
		return parseRSNX(ie)
	case len(ie) >= 1 && ie[0] == eidWAPI:
		return Data{}, 0
	}
	return parseWPA(ie)
}

func parseRSN(ie []byte) (Data, int) {
	d := Data{
		Proto:           ProtoRSN,
		PairwiseCipher:  CipherCCMP,
		GroupCipher:     CipherCCMP,
		KeyMgmt:         KeyMgmtIEEE8021X,
		MgmtGroupCipher: CipherAES128CMAC,
	}
	if len(ie) < 4 {
		return d, -1
	}
	if int(ie[1]) != len(ie)-2 || le16(ie[2:]) != 1 {
		return d, -2
	}
	p := ie[4:]
	if len(p) >= selectorLen {
		d.GroupCipher = rsnCipher(p)
		p = p[selectorLen:]
	} else if len(p) > 0 {
		return d, -3
	}
	var code int
	if d.PairwiseCipher, p, code = suiteList(p, d.PairwiseCipher, rsnCipher, -4, -5); code != 0 {
		return d, code
	}
	if d.KeyMgmt, p, code = suiteList(p, d.KeyMgmt, rsnKeyMgmt, -6, -7); code != 0 {
		return d, code
	}
	if len(p) >= 2 {
		d.Capabilities = int(le16(p))
		p = p[2:]
	}
	if len(p) >= 2 {
		n := int(le16(p))
		p = p[2:]
		if n > len(p)/pmkidLen {
			return d, -9
		}
		d.PMKID = p[:n*pmkidLen]
		p = p[n*pmkidLen:]
	}
	if len(p) >= selectorLen {
		d.MgmtGroupCipher = rsnCipher(p)
		if d.MgmtGroupCipher != CipherAES128CMAC {
			return d, -10
		}
	}
	return d, 0
}

func parseWPA(ie []byte) (Data, int) {
	d := Data{
		Proto:          ProtoWPA,
		PairwiseCipher: CipherTKIP,
		GroupCipher:    CipherTKIP,
		KeyMgmt:        KeyMgmtIEEE8021X,
	}
	if len(ie) < 8 {
		return d, -1
	}
	if ie[0] != eidVendorSpecific || int(ie[1]) != len(ie)-2 ||
		[3]byte(ie[2:5]) != ouiWPA || ie[5] != 1 || le16(ie[6:]) != 1 {
		return d, -2
	}
	p := ie[8:]
	if len(p) >= selectorLen {
		d.GroupCipher = wpaCipher(p)
		p = p[selectorLen:]
	} else if len(p) > 0 {
		return d, -3
	}
	var code int
	if d.PairwiseCipher, p, code = suiteList(p, d.PairwiseCipher, wpaCipher, -4, -5); code != 0 {
		return d, code
	}
	if d.KeyMgmt, p, code = suiteList(p, d.KeyMgmt, wpaKeyMgmt, -6, -7); code != 0 {
		return d, code
	}
	if len(p) >= 2 {
		d.Capabilities = int(le16(p))
	}
	return d, 0
}

// parseRSNX reads the first capability byte. A station without SAE H2E
// accepts all values.
func parseRSNX(ie []byte) (Data, int) {
	if len(ie) < 3 {
		return Data{}, -1
	}
	return Data{RSNXECapa: ie[2]}, 0
}

// suiteList reads a count and the suites. It keeps def when p is empty.
func suiteList(p []byte, def int, suite func([]byte) int, errCount, errShort int) (int, []byte, int) {
	if len(p) == 1 {
		return def, p, errShort
	}
	if len(p) < 2 {
		return def, p, 0
	}
	n := int(le16(p))
	p = p[2:]
	if n == 0 || len(p) < n*selectorLen {
		return 0, p, errCount
	}
	bits := 0
	for range n {
		bits |= suite(p)
		p = p[selectorLen:]
	}
	return bits, p, 0
}

func rsnCipher(s []byte) int {
	if [3]byte(s[:3]) != ouiRSN {
		return 0
	}
	switch s[3] {
	case 0:
		return CipherNone
	case 1:
		return CipherWEP40
	case 2:
		return CipherTKIP
	case 4:
		return CipherCCMP
	case 5:
		return CipherWEP104
	case 6:
		return CipherAES128CMAC
	}
	return 0
}

func rsnKeyMgmt(s []byte) int {
	if [3]byte(s[:3]) != ouiRSN {
		return 0
	}
	switch s[3] {
	case 1:
		return KeyMgmtIEEE8021X
	case 2:
		return KeyMgmtPSK
	case 5:
		return KeyMgmtIEEE8021XSHA256
	case 6:
		return KeyMgmtPSKSHA256
	case 8:
		return KeyMgmtSAE
	case 18:
		return KeyMgmtOWE
	case 24:
		return KeyMgmtSAEExtKey
	}
	return 0
}

func wpaCipher(s []byte) int {
	if [3]byte(s[:3]) != ouiWPA {
		return 0
	}
	switch s[3] {
	case 0:
		return CipherNone
	case 1:
		return CipherWEP40
	case 2:
		return CipherTKIP
	case 4:
		return CipherCCMP
	case 5:
		return CipherWEP104
	}
	return 0
}

func wpaKeyMgmt(s []byte) int {
	if [3]byte(s[:3]) != ouiWPA {
		return 0
	}
	switch s[3] {
	case 0:
		return KeyMgmtWPANone
	case 1:
		return KeyMgmtIEEE8021X
	case 2:
		return KeyMgmtPSK
	}
	return 0
}

// PublicCipher is cipher_type_map_supp_to_public of
// wpa_supplicant/src/rsn_supp/wpa.c. It gives a wifi_cipher_type_t.
func PublicCipher(c int) int {
	switch c {
	case CipherNone:
		return 0
	case CipherWEP40:
		return 1
	case CipherWEP104:
		return 2
	case CipherTKIP:
		return 3
	case CipherCCMP:
		return 4
	case CipherCCMP | CipherTKIP:
		return 5
	case CipherAES128CMAC:
		return 6
	}
	return 12 // WIFI_CIPHER_TYPE_UNKNOWN
}

func le16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
