package eapol

// A GTK KDE is element 0xdd with OUI 00-0F-AC, data type 1, then 2 bytes and the GTK
// (IEEE 802.11-2020 table 12-9 and figure 12-35). The first of the 2 bytes holds the key ID in bits 0 and 1.
const (
	eidVendor   = 0xdd
	kdeGTK      = 1
	gtkKDEFixed = 4 + 2 // OUI and type, then key ID and reserved
	maxGTKLen   = 32

	// The IGTK KDE (IEEE 802.11-2020 figure 12-42): OUI and type 9, a key
	// ID of 2 bytes, the IPN of 6 bytes and the IGTK of BIP-CMAC-128.
	kdeIGTK  = 9
	igtkLen  = 16
	ipnLen   = 6
	igtkBody = 4 + 2 + ipnLen + igtkLen
)

// findGTK returns the GTK and its key ID from decrypted key data. Key data ends at a 0xdd pad byte
// with a zero length (IEEE 802.11-2020 12.7.2, item j).
func findGTK(data []byte) ([]byte, uint8, error) {
	for len(data) >= 2 {
		id, n := data[0], int(data[1])
		if id == eidVendor && n == 0 {
			break
		}
		if len(data) < 2+n {
			return nil, 0, ErrKeyData
		}
		body := data[2 : 2+n]
		if id == eidVendor && n >= gtkKDEFixed && body[0] == 0x00 && body[1] == 0x0f && body[2] == 0xac && body[3] == kdeGTK {
			gtk := body[gtkKDEFixed:]
			if len(gtk) == 0 || len(gtk) > maxGTKLen {
				return nil, 0, ErrKeyData
			}
			return gtk, body[4] & 3, nil
		}
		data = data[2+n:]
	}
	return nil, 0, ErrNoGTK
}

// findIGTK returns the IGTK KDE body after the OUI and type: the key ID, the
// IPN and the IGTK. It gives nil when key data has no IGTK KDE.
func findIGTK(data []byte) []byte {
	for len(data) >= 2 {
		id, n := data[0], int(data[1])
		if id == eidVendor && n == 0 || len(data) < 2+n {
			return nil
		}
		body := data[2 : 2+n]
		if id == eidVendor && n == igtkBody && body[0] == 0x00 && body[1] == 0x0f && body[2] == 0xac && body[3] == kdeIGTK {
			return body[4:]
		}
		data = data[2+n:]
	}
	return nil
}

// findRSN returns the full RSN element (with its ID and length) from key data, or nil.
func findRSN(data []byte) []byte {
	for len(data) >= 2 {
		n := int(data[1])
		if len(data) < 2+n {
			return nil
		}
		if data[0] == 48 {
			return data[:2+n]
		}
		data = data[2+n:]
	}
	return nil
}
