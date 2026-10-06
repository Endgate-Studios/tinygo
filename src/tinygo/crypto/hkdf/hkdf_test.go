package hkdf

import (
	"encoding/hex"
	"testing"
)

func un(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestRFC5869 uses the test cases A.1, A.2 and A.3 of RFC 5869.
func TestRFC5869(t *testing.T) {
	long := func(from, n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(from + i)
		}
		return hex.EncodeToString(b)
	}
	for _, tt := range []struct {
		ikm, salt, info, prk, okm string
	}{
		{"0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b", "000102030405060708090a0b0c", "f0f1f2f3f4f5f6f7f8f9",
			"077709362c2e32df0ddc3f0dc47bba6390b6c73bb50f9c3122ec844ad7c2b3e5",
			"3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"},
		{long(0, 80), long(0x60, 80), long(0xb0, 80),
			"06a6b88c5853361a06104c9ceb35b45cef760014904671014a193f40c15fc244",
			"b11e398dc80327a1c8e7f78c596a49344f012eda2d4efad8a050cc4c19afa97c59045a99cac7827271cb41c65e590e09da3275600c2f09b8367793a9aca3db71cc30c58179ec3e87c14c01d5c1f3434f1d87"},
		{"0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b", "", "",
			"19ef24a32c717b167f33a91d6f648bdf96596776afdb6377ac434c1c293ccb04",
			"8da4e775a563c18f715f802a063c5a31b8a11f5c5ee1879ec3454e5f3c738d2d9d201395faa4b61a96c8"},
	} {
		var prk [Size]byte
		Extract(&prk, un(t, tt.salt), un(t, tt.ikm))
		if hex.EncodeToString(prk[:]) != tt.prk {
			t.Errorf("PRK %x, want %s", prk, tt.prk)
		}
		info := un(t, tt.info)
		okm := make([]byte, len(tt.okm)/2)
		Expand(okm, prk[:], info[:len(info)/2], info[len(info)/2:])
		if hex.EncodeToString(okm) != tt.okm {
			t.Errorf("OKM %x, want %s", okm, tt.okm)
		}
	}
}
