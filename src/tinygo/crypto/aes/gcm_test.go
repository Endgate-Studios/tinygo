package aes

import (
	"bytes"
	stdaes "crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math/rand"
	"testing"
)

func unhexT(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestGCMVectors uses test cases 3 and 4 of McGrew and Viega, "The Galois/
// Counter Mode of Operation", appendix B, with AES-128.
func TestGCMVectors(t *testing.T) {
	key := unhexT(t, "feffe9928665731c6d6a8f9467308308")
	nonce := [NonceSize]byte(unhexT(t, "cafebabefacedbaddecaf888"))
	plain := unhexT(t, "d9313225f88406e5a55909c5aff5269a86a7a9531534f7da2e4c303d8a318a721c3c0c95956809532fcf0e2449a6b525b16aedf5aa0de657ba637b391aafd255")
	for _, tt := range []struct {
		plainLen  int
		aad, want string
	}{
		{64, "", "42831ec2217774244b7221b784d0d49ce3aa212f2c02a4e035c17e2329aca12e21d514b25466931c7d8f6a5aac84aa051ba30b396a0aac973d58e091473f5985" + "4d5c2af327cd64a62cf35abd2ba6fab4"},
		{60, "feedfacedeadbeeffeedfacedeadbeefabaddad2", "42831ec2217774244b7221b784d0d49ce3aa212f2c02a4e035c17e2329aca12e21d514b25466931c7d8f6a5aac84aa051ba30b396a0aac973d58e091" + "5bc94fbc3221a5db94fae95ae7121a47"},
	} {
		c, err := New(newMemory(t), key)
		if err != nil {
			t.Fatal(err)
		}
		var g GCM
		g.Init(c)
		aad := unhexT(t, tt.aad)
		out := make([]byte, tt.plainLen+TagSize)
		g.Seal(out, &nonce, plain[:tt.plainLen], aad)
		if hex.EncodeToString(out) != tt.want {
			t.Errorf("seal of %d bytes\n got %x\nwant %s", tt.plainLen, out, tt.want)
		}
		back := make([]byte, tt.plainLen)
		if err := g.Open(back, &nonce, out, aad); err != nil || !bytes.Equal(back, plain[:tt.plainLen]) {
			t.Errorf("open: %v", err)
		}
		out[3] ^= 1
		if err := g.Open(back, &nonce, out, aad); err != ErrAuth {
			t.Errorf("changed ciphertext: %v, want ErrAuth", err)
		}
		c.Close()
	}
}

// TestGCMStd compares random messages with crypto/cipher, also in place.
func TestGCMStd(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 50; i++ {
		key := make([]byte, []int{16, 32}[i%2])
		r.Read(key)
		var nonce [NonceSize]byte
		r.Read(nonce[:])
		plain := make([]byte, r.Intn(100))
		aad := make([]byte, r.Intn(40))
		r.Read(plain)
		r.Read(aad)
		b, _ := stdaes.NewCipher(key)
		std, _ := cipher.NewGCM(b)
		want := std.Seal(nil, nonce[:], plain, aad)
		c, err := New(newMemory(t), key)
		if err != nil {
			t.Fatal(err)
		}
		var g GCM
		g.Init(c)
		buf := make([]byte, len(plain)+TagSize)
		copy(buf, plain)
		g.Seal(buf, &nonce, buf[:len(plain)], aad) // in place
		if !bytes.Equal(buf, want) {
			t.Fatalf("message %d differs from crypto/cipher", i)
		}
		if err := g.Open(buf, &nonce, buf, aad); err != nil || !bytes.Equal(buf[:len(plain)], plain) {
			t.Fatalf("open in place %d: %v", i, err)
		}
		c.Close()
	}
}

// ghashRef is the bit-serial multiplication of SP 800-38D algorithm 1.
func ghashRef(y, x, h *[BlockSize]byte) {
	vh := binary.BigEndian.Uint64(h[:8])
	vl := binary.BigEndian.Uint64(h[8:])
	xh := binary.BigEndian.Uint64(y[:8]) ^ binary.BigEndian.Uint64(x[:8])
	xl := binary.BigEndian.Uint64(y[8:]) ^ binary.BigEndian.Uint64(x[8:])
	var zh, zl uint64
	for i := 0; i < 128; i++ {
		var bit uint64
		if i < 64 {
			bit = xh >> (63 - i) & 1
		} else {
			bit = xl >> (127 - i) & 1
		}
		mask := -bit
		zh ^= vh & mask
		zl ^= vl & mask
		r := -(vl & 1)
		vl = vl>>1 | vh<<63
		vh = vh>>1 ^ 0xe100000000000000&r
	}
	binary.BigEndian.PutUint64(y[:8], zh)
	binary.BigEndian.PutUint64(y[8:], zl)
}

func TestGHASHBlock(t *testing.T) {
	var seed [32]byte
	for i := 0; i < 2000; i++ {
		var h, y, x [BlockSize]byte
		sum := sha256.Sum256(append(seed[:], byte(i), byte(i>>8)))
		copy(h[:], sum[:16])
		copy(y[:], sum[16:])
		sum = sha256.Sum256(sum[:])
		copy(x[:], sum[:16])
		switch i {
		case 0:
			h = [BlockSize]byte{}
		case 1:
			for k := range h {
				h[k], y[k], x[k] = 0xff, 0xff, 0
			}
		case 2:
			h, y, x = [BlockSize]byte{0x80}, [BlockSize]byte{15: 1}, [BlockSize]byte{}
		}
		want := y
		ghashRef(&want, &x, &h)
		g := GCM{h: load(&h)}
		yw, xw := load(&y), load(&x)
		for k := range yw {
			yw[k] ^= xw[k]
		}
		g.mul(&yw)
		if got := yw; got != load(&want) {
			t.Fatalf("case %d: %x, want %x", i, got, load(&want))
		}
	}
}
