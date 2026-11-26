//go:build esp32p4 && p4flash

package machine

import (
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

// Flash storage on the ESP32-P4, by the ROM's routines, with the p4flash tag. The whole chip is mapped
// at 0x41000000, for reads, once it's first used. A write suspends the L2 cache that
// reaches flash and PSRAM, so tinygo_flashCall runs in RAM, on a stack of its own, with the other core
// parked. See ESP-IDF spi_flash/cache_utils.c, spi_flash_disable_cache.

// ROM functions, by address, for tinygo_flashCall. See ESP-IDF esp_rom/esp32p4/ld/esp32p4.rom.ld.
const (
	romFlashEraseSector = 0x4fc0014c
	romFlashWrite       = 0x4fc00154
	romFlashRead        = 0x4fc00158
	romFlashUnlock      = 0x4fc0015c
)

const (
	flashImageOffset = 0x2000 // where the ROM looks for the image
	flashMMUIndex    = 0x5008C380
	flashMMUContent  = 0x5008C37C
	flashMMUValid    = 0x1000     // SOC_MMU_FLASH_VALID, as in esp32p4.S
	flashProgram     = 0x40010000 // where the program's mapped, from flash at _flash_map_addr
	flashVirtual     = 0x41000000 // where the whole chip's mapped, by address on the chip
	flashMMUFirst    = (flashVirtual - 0x40000000) / flashPageSize
	flashPageSize    = 0x10000
	flashMaxSize     = 16 << 20

	cacheMapL1Data = 1 << 4 // CACHE_MAP_L1_DCACHE in esp_rom/include/esp32p4/rom/cache.h
	cacheMapL2     = 1 << 5 // CACHE_MAP_L2_CACHE
)

//export tinygo_flashCall
func flashCall(fn uintptr, a, b, c uint32, stack uintptr) int32

//export Cache_Invalidate_Addr
func cacheInvalidateAddr(cacheMap, addr, size uint32) int32

//go:linkname flashSafeEnter runtime.esp32p4EnterFlashSafeSection
func flashSafeEnter() (interrupt.State, bool)

//go:linkname flashSafeExit runtime.esp32p4ExitFlashSafeSection
func flashSafeExit(state interrupt.State, multicore bool)

//go:extern __flash_data_phys
var flashDataPhys [0]byte

//go:extern _flash_map_addr
var flashMapAddr uint32

//go:extern _flash_end
var flashProgramEnd [0]byte

// romSpiflashChip is esp_rom_spiflash_chip_t in ESP-IDF esp_rom/include/esp_rom_spiflash.h.
type romSpiflashChip struct {
	deviceID   uint32
	chipSize   uint32
	blockSize  uint32
	sectorSize uint32
	pageSize   uint32
	statusMask uint32
}

//go:extern rom_spiflash_legacy_data
var romSpiflashLegacyData *romSpiflashChip

// Internal RAM, as package variables are, for while PSRAM can't be reached. One at a time, as the
// flash-safe section is.
var (
	flashCallStack [1024]uint32
	rawFlashBuf    [rawFlashChunk / 4]uint32
	flashReady     bool
)

const rawFlashChunk = 1024

func flashROM(fn uintptr, a, b, c uint32) int32 {
	return flashCall(fn, a, b, c, uintptr(unsafe.Pointer(&flashCallStack[0]))+unsafe.Sizeof(flashCallStack))
}

// flashSetup tells the ROM the chip's geometry, and maps the chip. The image header has the chip's
// size, which espflasher sets. The flash-safe section must be held.
func flashSetup() {
	if flashReady {
		return
	}
	flashReady = true
	chip := romSpiflashLegacyData
	chip.blockSize, chip.sectorSize, chip.pageSize, chip.statusMask = 64<<10, 4<<10, 256, 0xffff
	chip.chipSize = flashMaxSize
	if flashROM(romFlashRead, flashImageOffset, uint32(uintptr(unsafe.Pointer(&rawFlashBuf[0]))), 4) == 0 {
		header := rawFlashBuf[0]
		if byte(header) == 0xE9 && header>>28 <= 4 {
			chip.chipSize = 1 << 20 << (header >> 28)
		}
	}

	index := (*volatile.Register32)(unsafe.Pointer(uintptr(flashMMUIndex)))
	content := (*volatile.Register32)(unsafe.Pointer(uintptr(flashMMUContent)))
	for page := uint32(0); page < flashMaxSize/flashPageSize; page++ {
		index.Set(flashMMUFirst + page)
		content.Set(page | flashMMUValid)
	}
	flashInvalidate(0, flashMaxSize)
}

// flashInvalidate drops what the caches have of a range of the chip, at its mapping.
func flashInvalidate(phys, size uint32) {
	cacheInvalidateAddr(cacheMapL1Data, flashVirtual+phys, size)
	cacheInvalidateAddr(cacheMapL2, flashVirtual+phys, size)
}

// flashWritable says a range is past the running program.
func flashWritable(addr, size uint32) bool {
	end := flashMapAddr + uint32(uintptr(unsafe.Pointer(&flashProgramEnd))-flashProgram)
	return addr >= end && addr+size <= romSpiflashLegacyData.chipSize
}

var _ BlockDevice = flashBlockDevice{}

type flashBlockDevice struct{}

// Flash is the flash after the program.
var Flash flashBlockDevice

const writeBlockSize = 4
const eraseBlockSizeValue = 1 << 12

func eraseBlockSize() int64 { return eraseBlockSizeValue }

func flashEnd() uintptr {
	state, multicore := flashSafeEnter()
	flashSetup()
	flashSafeExit(state, multicore)
	return FlashDataStart() + uintptr(romSpiflashLegacyData.chipSize) - uintptr(unsafe.Pointer(&flashDataPhys))
}

// ReadAt reads the given number of bytes from the block device.
func (f flashBlockDevice) ReadAt(p []byte, off int64) (n int, err error) {
	if FlashDataStart()+uintptr(off)+uintptr(len(p)) > flashEnd() {
		return 0, errFlashCannotReadPastEOF
	}
	copy(p, unsafe.Slice((*byte)(unsafe.Pointer(FlashDataStart()+uintptr(off))), len(p)))
	return len(p), nil
}

// WriteAt writes the given number of bytes to the block device, already erased, padding the last word
// with 0xFF. It must not be called from an interrupt or with interrupts disabled.
func (f flashBlockDevice) WriteAt(p []byte, off int64) (n int, err error) {
	if FlashDataStart()+uintptr(off)+uintptr(len(p)) > flashEnd() {
		return 0, errFlashCannotWritePastEOF
	}
	padded := flashPad(p, writeBlockSize)
	phys := uint32(uintptr(unsafe.Pointer(&flashDataPhys))) + uint32(off)
	if err := RawFlash.WriteAt(padded, phys); err != nil {
		return 0, err
	}
	return len(padded), nil
}

// Size returns the number of bytes in this block device.
func (f flashBlockDevice) Size() int64 { return int64(flashEnd() - FlashDataStart()) }

// WriteBlockSize returns the block size in which data can be written.
func (f flashBlockDevice) WriteBlockSize() int64 { return writeBlockSize }

// EraseBlockSize returns the smallest erasable area.
func (f flashBlockDevice) EraseBlockSize() int64 { return eraseBlockSizeValue }

// EraseBlocks erases blocks by number. It must not be called from an interrupt or with interrupts disabled.
func (f flashBlockDevice) EraseBlocks(start, length int64) error {
	if FlashDataStart()+uintptr((start+length)*eraseBlockSizeValue) > flashEnd() {
		return errFlashCannotErasePastEOF
	}
	phys := uint32(uintptr(unsafe.Pointer(&flashDataPhys))) + uint32(start*eraseBlockSizeValue)
	for i := int64(0); i < length; i++ {
		if err := RawFlash.EraseSector(phys + uint32(i*eraseBlockSizeValue)); err != nil {
			return err
		}
	}
	return nil
}

type rawFlash struct{}

// RawFlash is the flash by address on the chip. It refuses to write the running program.
var RawFlash rawFlash

// ReadAt reads from an address on the chip.
func (rawFlash) ReadAt(p []byte, addr uint32) error {
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&rawFlashBuf[0])), rawFlashChunk)
	for len(p) > 0 {
		n := min(len(p), rawFlashChunk)
		state, multicore := flashSafeEnter()
		flashSetup()
		res := flashROM(romFlashRead, addr, uint32(uintptr(unsafe.Pointer(&rawFlashBuf[0]))), uint32((n+3)&^3))
		if res == 0 {
			copy(p, bytes[:n])
		}
		flashSafeExit(state, multicore)
		if res != 0 {
			return errFlashCannotReadPastEOF
		}
		p, addr = p[n:], addr+uint32(n)
	}
	return nil
}

// WriteAt writes whole words to an address on the chip, already erased.
func (rawFlash) WriteAt(p []byte, addr uint32) error {
	if len(p)%4 != 0 || addr%4 != 0 {
		return errFlashInvalidWriteLength
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&rawFlashBuf[0])), rawFlashChunk)
	for len(p) > 0 {
		n := min(len(p), rawFlashChunk)
		// p can be in PSRAM, which can't be reached during the write, so it's copied first.
		state, multicore := flashSafeEnter()
		flashSetup()
		if !flashWritable(addr, uint32(n)) {
			flashSafeExit(state, multicore)
			return errFlashCannotWritePastEOF
		}
		copy(bytes, p[:n])
		flashROM(romFlashUnlock, 0, 0, 0)
		res := flashROM(romFlashWrite, addr, uint32(uintptr(unsafe.Pointer(&rawFlashBuf[0]))), uint32(n))
		flashInvalidate(addr, uint32(n))
		flashSafeExit(state, multicore)
		if res != 0 {
			return errFlashCannotWriteData
		}
		p, addr = p[n:], addr+uint32(n)
	}
	return nil
}

// EraseSector erases the 4 KB sector at an address on the chip.
func (rawFlash) EraseSector(addr uint32) error {
	state, multicore := flashSafeEnter()
	defer flashSafeExit(state, multicore)
	flashSetup()
	if !flashWritable(addr, eraseBlockSizeValue) {
		return errFlashCannotErasePastEOF
	}
	flashROM(romFlashUnlock, 0, 0, 0)
	if flashROM(romFlashEraseSector, addr/eraseBlockSizeValue, 0, 0) != 0 {
		return errFlashCannotErasePage
	}
	flashInvalidate(addr, eraseBlockSizeValue)
	return nil
}
