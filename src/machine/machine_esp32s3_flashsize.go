//go:build esp32s3

package machine

import (
	"device/esp"
	"unsafe"
)

// flashChipSize asks the chip for its size, the third byte of its JEDEC ID being
// log2 of that. It's 0 if the answer isn't believable, and past 16 MB it's 16 MB,
// as addresses there need another mode. The flash-safe section must be held.
func flashChipSize() uint32 {
	esp.SPI1.W0.Set(0)
	esp.SPI1.CMD.Set(esp.SPI1_CMD_FLASH_RDID)
	for i := 0; esp.SPI1.CMD.Get() != 0; i++ {
		if i == 1000000 {
			return 0
		}
	}
	id := esp.SPI1.W0.Get()
	maker, capacity := id&0xff, id>>16&0xff
	if maker == 0 || maker == 0xff || capacity < 20 || capacity > 26 {
		return 0
	}
	return 1 << min(capacity, 24)
}

// flashEnd is the end of the flash data area on this chip: the linker maps as much
// as the largest chip could have.
func flashEnd() uintptr {
	if !flashSizeSet {
		state, multicore := flashSafeEnter()
		setFlashSize()
		flashSafeExit(state, multicore)
	}
	phys := uintptr(unsafe.Pointer(&flashDataPhys))
	size := uintptr(romSpiflashLegacyData.chipSize)
	if size <= phys {
		return FlashDataStart()
	}
	return min(FlashDataStart()+size-phys, FlashDataEnd())
}
