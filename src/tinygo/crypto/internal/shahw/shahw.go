//go:build esp32s3

// Package shahw drives the SHA accelerator of the ESP32-S3 in typical mode, as
// sha_ll.h of ESP-IDF v5.3.2. sha1 and sha256 share it, so it has one lock.
package shahw

import (
	"device/esp"
	"encoding/binary"
	"runtime/volatile"
	"sync"
	"unsafe"
)

// Modes of SHA_MODE (esp_sha_type of ESP-IDF).
const (
	SHA1   = 0
	SHA256 = 2
)

var (
	mu   sync.Mutex
	on   bool
	hMem = (*[8]volatile.Register32)(unsafe.Pointer(&esp.SHA.H_MEM[0]))
	mMem = (*[16]volatile.Register32)(unsafe.Pointer(&esp.SHA.M_MEM[0]))
)

// Block updates state, 20 bytes for SHA1 and 32 for SHA256, with one block.
// CONTINUE starts from the state in H_MEM. The registers hold the bytes in
// memory order.
func Block(mode uint32, state []byte, p *[64]byte) {
	mu.Lock()
	if !on {
		esp.SYSTEM.PERIP_CLK_EN1.SetBits(esp.SYSTEM_PERIP_CLK_EN1_CRYPTO_SHA_CLK_EN)
		esp.SYSTEM.PERIP_RST_EN1.SetBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_SHA_RST)
		// sha_ll_reset_register: DS and HMAC in reset also hold SHA in reset.
		esp.SYSTEM.PERIP_RST_EN1.ClearBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_SHA_RST |
			esp.SYSTEM_PERIP_RST_EN1_CRYPTO_DS_RST | esp.SYSTEM_PERIP_RST_EN1_CRYPTO_HMAC_RST)
		on = true
	}
	words := len(state) / 4
	for i := 0; i < words; i++ {
		hMem[i].Set(binary.LittleEndian.Uint32(state[4*i:]))
	}
	for i := range mMem {
		mMem[i].Set(binary.LittleEndian.Uint32(p[4*i:]))
	}
	esp.SHA.MODE.Set(mode)
	esp.SHA.CONTINUE.Set(1)
	for esp.SHA.BUSY.Get() != 0 {
	}
	for i := 0; i < words; i++ {
		binary.LittleEndian.PutUint32(state[4*i:], hMem[i].Get())
	}
	mu.Unlock()
}
