//go:build esp32s3

package sha512

import "tinygo/crypto/internal/shahw"

// Block updates s with one block on the SHA accelerator. A SHA-384 state is
// the SHA-512 state with other initial values, so mode SHA-512 does both.
func Block(s *State, p *[BlockSize]byte) { shahw.Block(shahw.SHA512, s[:], p[:]) }
