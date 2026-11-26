//go:build esp32s3

package radio

import (
	"runtime/memory"
	"unsafe"

	"tinygo/crypto/aes"
	"tinygo/net/wifi/wpa"
)

// cryptoPBKDF2SHA1 is the pbkdf2_sha1 entry of wpa_crypto_funcs_t. The blobs
// call it in esp_wifi_set_config for the PMK of the access point.
//
//export tinygo_crypto_pbkdf2_sha1
func cryptoPBKDF2SHA1(passphrase *byte, ssid *byte, ssidLen uint32, iterations int32, buf *byte, bufLen uint32) int32 {
	if iterations != 4096 || bufLen > wpa.PMKLen {
		return -1
	}
	p := unsafe.Slice(passphrase, 64) // password[64] of wifi_ap_config_t
	n := 0
	for n < len(p) && p[n] != 0 {
		n++
	}
	keys, err := wpa.New(blobHeap)
	if err != nil {
		return -1
	}
	defer keys.Close()
	if keys.SetPassphrase(p[:n], unsafe.Slice(ssid, ssidLen)) != nil {
		return -1
	}
	copy(unsafe.Slice(buf, bufLen), keys.PMK())
	return 0
}

// cryptoAESInit is the aes_encrypt_init and aes_decrypt_init entry. The
// context is a Cipher in the blob heap. ESP-NOW uses it for encrypted peers.
//
//export tinygo_crypto_aes_init
func cryptoAESInit(key *byte, n uint32) unsafe.Pointer {
	cs := memory.MakeSlice[aes.Cipher](blobHeap, 1)
	if cs == nil {
		return nil
	}
	if cs[0].Init(blobHeap, unsafe.Slice(key, n)) != nil {
		memory.FreeSlice(blobHeap, cs)
		return nil
	}
	return unsafe.Pointer(&cs[0])
}

// cryptoAESDeinit is the aes_encrypt_deinit and aes_decrypt_deinit entry.
//
//export tinygo_crypto_aes_deinit
func cryptoAESDeinit(ctx unsafe.Pointer) {
	if ctx == nil {
		return
	}
	(*aes.Cipher)(ctx).Close()
	blobHeap.Free(ctx)
}

//export tinygo_crypto_aes_encrypt
func cryptoAESEncrypt(ctx unsafe.Pointer, plain, crypt *byte) {
	(*aes.Cipher)(ctx).Encrypt((*[aes.BlockSize]byte)(unsafe.Pointer(crypt)), (*[aes.BlockSize]byte)(unsafe.Pointer(plain)))
}

//export tinygo_crypto_aes_decrypt
func cryptoAESDecrypt(ctx unsafe.Pointer, crypt, plain *byte) {
	(*aes.Cipher)(ctx).Decrypt((*[aes.BlockSize]byte)(unsafe.Pointer(plain)), (*[aes.BlockSize]byte)(unsafe.Pointer(crypt)))
}
