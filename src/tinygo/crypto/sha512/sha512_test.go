package sha512

import (
	stdsha512 "crypto/sha512"
	"testing"
)

func TestDigest(t *testing.T) {
	msg := make([]byte, 400)
	for i := range msg {
		msg[i] = byte(i * 13)
	}
	for n := 0; n <= len(msg); n++ {
		var d Digest
		d.Reset(Init512, 0)
		d.Write(msg[:n/3])
		d.Write(msg[n/3 : n])
		var got [Size]byte
		d.Sum(&got)
		if want := stdsha512.Sum512(msg[:n]); got != want {
			t.Fatalf("SHA-512 of %d bytes: %x, want %x", n, got, want)
		}
		var g384 [Size384]byte
		Sum384(&g384, msg[:n])
		if want := stdsha512.Sum384(msg[:n]); g384 != want {
			t.Fatalf("SHA-384 of %d bytes: %x, want %x", n, g384, want)
		}
	}
}
