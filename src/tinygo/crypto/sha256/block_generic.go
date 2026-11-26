//go:build !esp32s3

package sha256

// Block updates s with one block.
func Block(s *State, p *[BlockSize]byte) { blockSoft(s, p) }
