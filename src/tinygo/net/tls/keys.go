package tls

import (
	"tinygo/crypto/aes"
	"tinygo/crypto/hkdf"
	"tinygo/crypto/sha256"
)

// The key schedule of RFC 8446 section 7.1 for TLS_AES_128_GCM_SHA256.

const (
	keyLen = 16
	ivLen  = aes.NonceSize
)

// expandLabel is HKDF-Expand-Label: the info is the length, "tls13 " and the
// label, and the context, each with its length.
func expandLabel(out, secret []byte, label string, context []byte) {
	var info [2 + 1 + 6 + 32 + 1 + sha256.Size]byte
	n := len(out)
	info[0], info[1] = byte(n>>8), byte(n)
	info[2] = byte(6 + len(label))
	k := 3 + copy(info[3:], "tls13 ")
	k += copy(info[k:], label)
	info[k] = byte(len(context))
	k++
	k += copy(info[k:], context)
	hkdf.Expand(out, secret, info[:k], nil)
	clear(info[:])
}

// deriveSecret is Derive-Secret with the transcript hash th.
func deriveSecret(out *[sha256.Size]byte, secret []byte, label string, th []byte) {
	expandLabel(out[:], secret, label, th)
}

// emptyHash is the SHA-256 of the empty string.
var emptyHash = [sha256.Size]byte{
	0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24,
	0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55,
}

// trafficKeys is the AEAD of one direction (section 7.3).
type trafficKeys struct {
	cipher aes.Cipher
	gcm    aes.GCM
	iv     [ivLen]byte
	seq    uint64
	on     bool
}

// set derives the key and the IV of a traffic secret and starts the
// sequence number at 0.
func (t *trafficKeys) set(mem aes.Memory, secret []byte) error {
	t.close()
	var key [keyLen]byte
	expandLabel(key[:], secret, "key", nil)
	expandLabel(t.iv[:], secret, "iv", nil)
	err := t.cipher.Init(mem, key[:])
	clear(key[:])
	if err != nil {
		return err
	}
	t.gcm.Init(&t.cipher)
	t.seq, t.on = 0, true
	return nil
}

// nonce is the IV XOR the 64-bit sequence number (section 5.3).
func (t *trafficKeys) nonce() [ivLen]byte {
	n := t.iv
	for i := 0; i < 8; i++ {
		n[ivLen-1-i] ^= byte(t.seq >> (8 * i))
	}
	return n
}

func (t *trafficKeys) close() {
	if t.on {
		t.gcm.Clear()
		t.cipher.Close()
	}
	clear(t.iv[:])
	t.on = false
}
