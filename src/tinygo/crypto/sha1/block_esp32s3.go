//go:build esp32s3

package sha1

import "tinygo/crypto/internal/shahw"

// Block updates s with one block on the SHA accelerator.
func Block(s *State, p *[BlockSize]byte) { shahw.Block(shahw.SHA1, s[:], p[:]) }
