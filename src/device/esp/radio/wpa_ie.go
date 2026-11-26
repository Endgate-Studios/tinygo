//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"device/esp/radio/ie"
	"unsafe"
)

// wpaParseIE is wpa_parse_wpa_ie_wrapper of ESP-IDF v5.3.2
// esp_supplicant/src/esp_wpa_main.c. The blobs call it for the WPA and RSN
// elements of each beacon.
//
//export tinygo_wifi_parse_wpa_ie
func wpaParseIE(p *byte, n uintptr, out *C.wifi_wpa_ie_t) int32 {
	d, code := ie.Parse(unsafe.Slice(p, n))
	out.proto = C.int(d.Proto)
	out.pairwise_cipher = C.int(ie.PublicCipher(d.PairwiseCipher))
	out.group_cipher = C.int(ie.PublicCipher(d.GroupCipher))
	out.key_mgmt = C.int(d.KeyMgmt)
	out.capabilities = C.int(d.Capabilities)
	out.num_pmkid = C.size_t(len(d.PMKID) / 16)
	out.pmkid = nil
	if len(d.PMKID) > 0 {
		out.pmkid = (*C.uint8_t)(unsafe.Pointer(&d.PMKID[0]))
	}
	out.mgmt_group_cipher = C.int(ie.PublicCipher(d.MgmtGroupCipher))
	out.rsnxe_capa = C.uint8_t(d.RSNXECapa)
	return int32(code)
}
