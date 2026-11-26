package sha1

import (
	stdsha1 "crypto/sha1"
	"testing"
)

func TestDigest(t *testing.T) {
	msg := make([]byte, 300)
	for i := range msg {
		msg[i] = byte(i * 7)
	}
	for n := 0; n <= len(msg); n++ {
		var d Digest
		d.Reset(Init, 0)
		// Write in two parts, to cross the block buffer.
		d.Write(msg[:n/3])
		d.Write(msg[n/3 : n])
		var got [Size]byte
		d.Sum(&got)
		if want := stdsha1.Sum(msg[:n]); got != want {
			t.Fatalf("length %d: %x, want %x", n, got, want)
		}
	}
}

// TestSavedState continues a message from the state after its first block,
// as HMAC does with the ipad block.
func TestSavedState(t *testing.T) {
	msg := make([]byte, BlockSize+20)
	for i := range msg {
		msg[i] = byte(i ^ 0x36)
	}
	s := Init
	Block(&s, (*[BlockSize]byte)(msg[:BlockSize]))
	var d Digest
	d.Reset(s, BlockSize)
	d.Write(msg[BlockSize:])
	var got [Size]byte
	d.Sum(&got)
	if want := stdsha1.Sum(msg); got != want {
		t.Fatalf("%x, want %x", got, want)
	}
}
