//go:build esp32s3

package sha1

import (
	"device/esp"
	"encoding/binary"
	"runtime/volatile"
	"sync"
	"unsafe"
)

// The SHA accelerator in typical mode, as sha_ll.h of ESP-IDF v5.3.2 for the
// ESP32-S3. CONTINUE starts from the state in H_MEM.

const modeSHA1 = 0

var (
	hwMu sync.Mutex
	hwOn bool
	hMem = (*[Size / 4]volatile.Register32)(unsafe.Pointer(&esp.SHA.H_MEM[0]))
	mMem = (*[BlockSize / 4]volatile.Register32)(unsafe.Pointer(&esp.SHA.M_MEM[0]))
)

// Block updates s with one block.
func Block(s *State, p *[BlockSize]byte) {
	hwMu.Lock()
	if !hwOn {
		esp.SYSTEM.PERIP_CLK_EN1.SetBits(esp.SYSTEM_PERIP_CLK_EN1_CRYPTO_SHA_CLK_EN)
		esp.SYSTEM.PERIP_RST_EN1.SetBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_SHA_RST)
		// sha_ll_reset_register: DS and HMAC in reset also hold SHA in reset.
		esp.SYSTEM.PERIP_RST_EN1.ClearBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_SHA_RST |
			esp.SYSTEM_PERIP_RST_EN1_CRYPTO_DS_RST | esp.SYSTEM_PERIP_RST_EN1_CRYPTO_HMAC_RST)
		hwOn = true
	}
	// The registers hold the bytes in memory order, as the buffers do.
	for i := range hMem {
		hMem[i].Set(binary.LittleEndian.Uint32(s[4*i:]))
	}
	for i := range mMem {
		mMem[i].Set(binary.LittleEndian.Uint32(p[4*i:]))
	}
	esp.SHA.MODE.Set(modeSHA1)
	esp.SHA.CONTINUE.Set(1)
	for esp.SHA.BUSY.Get() != 0 {
	}
	for i := range hMem {
		binary.LittleEndian.PutUint32(s[4*i:], hMem[i].Get())
	}
	hwMu.Unlock()
}
