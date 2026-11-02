package hkdf

import (
	"encoding/hex"
	"testing"
)

// Vectors from a Python HKDF-SHA512 on hmac and hashlib, including the
// salts and infos HomeKit pairing uses.
var vectors512 = []struct{ salt, ikm, info, okm string }{
	{"436f6e74726f6c2d53616c74", "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f", "436f6e74726f6c2d526561642d456e6372797074696f6e2d4b6579", "c2f56f1912e2fc1f6604fd5bccab619272f687345aa4bce1c3c857421bd3b821"},
	{"506169722d5665726966792d456e63727970742d53616c74", "4242424242424242424242424242424242424242424242424242424242424242", "506169722d5665726966792d456e63727970742d496e666f", "3d1273d0ed20387e67108cbe04db02eef26a431a23d104ec2b83cdf9cd400cae"},
	{"", "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b", "", "f5fa02b18298a72a8c23898a8703472c6eb179dc204c03425c970e3b164bf90fff22d04836d0e2343bac"},
	{"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f", "606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeaf", "b0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff", "01c6270d475708afd9e9a6ff478e92006371425c2a62e24e7bf7b565ee4515bd048fed81dc602b806d3dac6da8a9c833210fa05c34baffeb7212ceced52b10b5ce813890de188b0230e0927179273dd24fcc3f303014cc3cc4a259911ec76cf9f05d97258744b1f271ee5b807cf6da1b83071013b972f22ee426706b9e3a5397af0bf4b7708e779e954571f0ad0317a19e9a28f66b9f7d98bcb79acae796b52db757c8ec234866c41fb44e05737252748f8ab5250d966de6e32b16e4a85a3f8e175bff262e6ffb27"},
}

func TestKey512(t *testing.T) {
	for i, v := range vectors512 {
		salt, _ := hex.DecodeString(v.salt)
		ikm, _ := hex.DecodeString(v.ikm)
		info, _ := hex.DecodeString(v.info)
		want, _ := hex.DecodeString(v.okm)
		out := make([]byte, len(want))
		Key512(out, salt, ikm, info)
		if got := hex.EncodeToString(out); got != v.okm {
			t.Errorf("vector %d: Key512 = %s, want %s", i, got, v.okm)
		}
	}
}

// Key512 runs once per pairing, so a few small allocations are fine; this
// keeps them from growing unnoticed.
func TestKey512Allocations(t *testing.T) {
	var out [32]byte
	salt, ikm, info := []byte("Control-Salt"), make([]byte, 64), []byte("Control-Read-Encryption-Key")
	if n := testing.AllocsPerRun(20, func() { Key512(out[:], salt, ikm, info) }); n > 2 {
		t.Errorf("Key512 allocates %v times, want at most 2", n)
	}
}
