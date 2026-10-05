package ie

import "testing"

// FuzzParse gives elements to the parser. No element may make it panic.
func FuzzParse(f *testing.F) {
	f.Add([]byte{48, 20, 1, 0, 0, 0x0f, 0xac, 4, 1, 0, 0, 0x0f, 0xac, 4, 1, 0, 0, 0x0f, 0xac, 2, 0x0c, 0})
	f.Add([]byte{0xdd, 22, 0x00, 0x50, 0xf2, 1, 1, 0, 0x00, 0x50, 0xf2, 2, 1, 0, 0x00, 0x50, 0xf2, 2, 1, 0, 0x00, 0x50, 0xf2, 2})
	f.Add([]byte{244, 1, 0x20})
	f.Fuzz(func(t *testing.T, e []byte) {
		Parse(e)
	})
}
