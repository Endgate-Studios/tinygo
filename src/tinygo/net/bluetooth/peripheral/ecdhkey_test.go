package peripheral

import (
	"crypto/ecdh"
	"io"
)

// ecdhKey makes a crypto/ecdh P-256 key from rng. ecdh.GenerateKey ignores its reader since Go 1.24
// and reads crypto/rand, which the ESP32-S3 target does not have, so the tests build keys from bytes.
func ecdhKey(rng io.Reader) (*ecdh.PrivateKey, error) {
	var b [32]byte
	for {
		if _, err := io.ReadFull(rng, b[:]); err != nil {
			return nil, err
		}
		if k, err := ecdh.P256().NewPrivateKey(b[:]); err == nil {
			return k, nil
		}
	}
}
