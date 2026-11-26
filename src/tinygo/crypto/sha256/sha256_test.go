package sha256

import (
	"bytes"
	"crypto/hmac"
	stdsha256 "crypto/sha256"
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
		if want := stdsha256.Sum256(msg[:n]); got != want {
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
	if want := stdsha256.Sum256(msg); got != want {
		t.Fatalf("%x, want %x", got, want)
	}
}

func TestHMAC(t *testing.T) {
	for _, keyLen := range []int{0, 20, 64, 65, 131} {
		key := make([]byte, keyLen)
		for i := range key {
			key[i] = byte(i + 1)
		}
		msg := []byte("The quick brown fox jumps over the lazy dog, more than one block long..")
		var h HMAC
		h.Init(key)
		var got [Size]byte
		h.Sum(&got, msg[:10], msg[10:40], nil, msg[40:])
		ref := hmac.New(stdsha256.New, key)
		ref.Write(msg)
		if want := ref.Sum(nil); !bytes.Equal(got[:], want) {
			t.Fatalf("key of %d bytes: %x, want %x", keyLen, got, want)
		}
	}
}
