//go:build esp32s3

package aes

import (
	"device/esp"
	"encoding/binary"
	"runtime/volatile"
	"sync"
	"unsafe"
)

// The AES accelerator in typical mode, as aes_ll.h and aes_hal.c of ESP-IDF
// v5.3.2 for the ESP32-S3. It has no AES-192, so that key stays in software.

const modeDecrypt = 4 // AES_MODE: 0 and 2 encrypt with 128 and 256 bit keys

var (
	hwMu    sync.Mutex
	hwOn    bool
	keyRegs = (*[8]volatile.Register32)(unsafe.Pointer(&esp.AES.KEY_0))
	inRegs  = (*[4]volatile.Register32)(unsafe.Pointer(&esp.AES.TEXT_IN_0))
	outRegs = (*[4]volatile.Register32)(unsafe.Pointer(&esp.AES.TEXT_OUT_0))
)

func (c *Cipher) encrypt(dst, src *[BlockSize]byte) {
	if c.rounds == 12 {
		c.encryptSoft(dst, src)
		return
	}
	c.hwBlock(dst, src, 0)
}

func (c *Cipher) decrypt(dst, src *[BlockSize]byte) {
	if c.rounds == 12 {
		c.decryptSoft(dst, src)
		return
	}
	c.hwBlock(dst, src, modeDecrypt)
}

// hwBlock is aes_hal_setkey and aes_hal_transform_block. The registers hold
// the bytes in memory order.
func (c *Cipher) hwBlock(dst, src *[BlockSize]byte, mode uint32) {
	hwStart(c.key(), mode)
	hwRun(dst, src)
	hwStop()
}

// ctr XORs src with the encrypted counter blocks from cb into dst, and
// increments the last 4 bytes of cb for each block. It sets the key once.
func (c *Cipher) ctr(dst, src []byte, cb *[BlockSize]byte) {
	if c.rounds == 12 {
		c.ctrSoft(dst, src, cb)
		return
	}
	var ks [BlockSize]byte
	hwStart(c.key(), 0)
	for len(src) > 0 {
		hwRun(&ks, cb)
		n := xorBlock(dst, src, &ks)
		dst, src = dst[n:], src[n:]
		incCounter(cb)
	}
	hwStop()
	clear(ks[:])
}

func hwStart(key []byte, mode uint32) {
	hwMu.Lock()
	if !hwOn {
		esp.SYSTEM.PERIP_CLK_EN1.SetBits(esp.SYSTEM_PERIP_CLK_EN1_CRYPTO_AES_CLK_EN)
		esp.SYSTEM.PERIP_RST_EN1.SetBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_AES_RST)
		// aes_ll_reset_register: DS in reset also holds AES in reset.
		esp.SYSTEM.PERIP_RST_EN1.ClearBits(esp.SYSTEM_PERIP_RST_EN1_CRYPTO_AES_RST |
			esp.SYSTEM_PERIP_RST_EN1_CRYPTO_DS_RST)
		esp.AES.DMA_ENABLE.Set(0)
		hwOn = true
	}
	esp.AES.MODE.Set(mode + uint32(len(key)/8-2))
	for i := 0; i < len(key)/4; i++ {
		keyRegs[i].Set(binary.LittleEndian.Uint32(key[4*i:]))
	}
}

func hwRun(dst, src *[BlockSize]byte) {
	for i := range inRegs {
		inRegs[i].Set(binary.LittleEndian.Uint32(src[4*i:]))
	}
	esp.AES.TRIGGER.Set(1)
	for esp.AES.STATE.Get() != 0 {
	}
	for i := range outRegs {
		binary.LittleEndian.PutUint32(dst[4*i:], outRegs[i].Get())
	}
}

// hwStop clears the key registers, which keep the key of the last block.
func hwStop() {
	for i := range keyRegs {
		keyRegs[i].Set(0)
	}
	hwMu.Unlock()
}
