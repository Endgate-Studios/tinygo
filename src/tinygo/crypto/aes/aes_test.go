package aes

import (
	stdaes "crypto/aes"
	"encoding/hex"
	"math/rand"
	"testing"

	"runtime/memory/arena"
)

func newMemory(t *testing.T) *arena.Heap {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 4096))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func block(t *testing.T, s string) (b [BlockSize]byte) {
	t.Helper()
	d, err := hex.DecodeString(s)
	if err != nil || len(d) != BlockSize {
		t.Fatalf("bad block %q", s)
	}
	copy(b[:], d)
	return b
}

func TestSbox(t *testing.T) {
	tests := []struct{ in, want byte }{
		{0x00, 0x63}, {0x01, 0x7c}, {0x53, 0xed}, {0xff, 0x16},
	}
	for _, tt := range tests {
		if got := sbox[tt.in]; got != tt.want {
			t.Errorf("sbox[%#x] = %#x, want %#x (FIPS 197 figure 7)", tt.in, got, tt.want)
		}
		if invSbox[tt.want] != tt.in {
			t.Errorf("invSbox[%#x] != %#x", tt.want, tt.in)
		}
	}
}

func TestFIPS197(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "C.1 AES-128", key: "000102030405060708090a0b0c0d0e0f", want: "69c4e0d86a7b0430d8cdb78070b4c55a"},
		{name: "C.2 AES-192", key: "000102030405060708090a0b0c0d0e0f1011121314151617", want: "dda97ca4864cdfe06eaf70a0ec0d7191"},
		{name: "C.3 AES-256", key: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f", want: "8ea2b7ca516745bfeafc49904b496089"},
	}
	plain := block(t, "00112233445566778899aabbccddeeff")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newMemory(t)
			key, _ := hex.DecodeString(tt.key)
			c, err := New(mem, key)
			if err != nil {
				t.Fatal(err)
			}
			var got [BlockSize]byte
			if err := c.Encrypt(&got, &plain); err != nil {
				t.Fatal(err)
			}
			if want := block(t, tt.want); got != want {
				t.Fatalf("Encrypt = %x, want %x", got, want)
			}
			if err := c.Decrypt(&got, &got); err != nil || got != plain {
				t.Fatalf("Decrypt = %x, want %x", got, plain)
			}
			c.Close()
			if n := mem.Stats().InUse; n != 0 {
				t.Errorf("memory in use after Close = %d, want 0", n)
			}
		})
	}
}

func TestMatchesStandardLibrary(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	mem := newMemory(t)
	for _, keyLen := range []int{16, 24, 32} {
		key := make([]byte, keyLen)
		rng.Read(key)
		c, err := New(mem, key)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := stdaes.NewCipher(key)
		for i := 0; i < 200; i++ {
			var in, got, want [BlockSize]byte
			rng.Read(in[:])
			c.Encrypt(&got, &in)
			ref.Encrypt(want[:], in[:])
			if got != want {
				t.Fatalf("key %d bytes: Encrypt(%x) = %x, want %x", keyLen, in, got, want)
			}
			c.Decrypt(&got, &want)
			if got != in {
				t.Fatalf("key %d bytes: Decrypt did not return the plaintext", keyLen)
			}
		}
		c.Close()
	}
	if n := mem.Stats().InUse; n != 0 {
		t.Errorf("memory in use = %d, want 0", n)
	}
}

func TestErrors(t *testing.T) {
	mem := newMemory(t)
	if _, err := New(nil, make([]byte, 16)); err != ErrNilMemory {
		t.Errorf("New(nil) error = %v, want ErrNilMemory", err)
	}
	if _, err := New(mem, make([]byte, 15)); err != ErrKeySize {
		t.Errorf("15-byte key error = %v, want ErrKeySize", err)
	}
	tiny, _ := arena.NewHeap(make([]byte, 64))
	if _, err := New(tiny, make([]byte, 16)); err != ErrNoMemory {
		t.Errorf("tiny heap error = %v, want ErrNoMemory", err)
	}
	c, _ := New(mem, make([]byte, 16))
	c.Close()
	c.Close()
	var b [BlockSize]byte
	if c.Encrypt(&b, &b) != ErrClosed || c.Decrypt(&b, &b) != ErrClosed {
		t.Error("use after Close did not return ErrClosed")
	}
}
