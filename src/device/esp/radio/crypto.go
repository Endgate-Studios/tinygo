//go:build esp32s3

package radio

import (
	"unsafe"

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
