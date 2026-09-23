//go:build esp32s3 && numa_psram_octal

// Shared PSRAM support for ESP32-S3: linker symbols, MMU/ROM-call helpers,
// and address-map documentation used by the Octal SPI driver
// (runtime_esp32_psram_octal.go, build tag numa_psram_octal). Everything
// protocol-specific - SPI1 command sequencing, chip ID and mode register
// parsing, SPI0 cache-phase configuration - lives there.
//
// Only Octal is supported. Every ESP32-S3 module carrying 8MB of PSRAM uses
// Octal SPI, so a Quad driver had no hardware to verify against and was
// dropped rather than shipped untested.
//
// Address map (esp-idf soc/ext_mem_defs.h; verified on hardware while
// bringing up the Octal driver):
//
//	0x600C5000  MMU table, 512 x 32-bit entries, one per 64KB page.
//	            A SINGLE table shared by IBUS (0x42000000 window) and DBUS
//	            (0x3C000000 window), indexed by linear address:
//	            entry = (vaddr & SOC_MMU_LINEAR_ADDR_MASK 0x1FFFFFF) >> 16.
//	            There is no ICache/DCache half-split: 0x3C800000 and
//	            0x42800000 both decode to entry 128.
//	            Entry format: bits[13:0] physical page number, bit 14 =
//	            invalid (SOC_MMU_INVALID), bit 15 = target type, 0 = flash,
//	            1 = PSRAM (SOC_MMU_ACCESS_SPIRAM).
//
//	Flash XIP takes the low entries: IROM starts at entry 0 and DROM
//	follows immediately after it (esp32s3.S, targets/esp32s3.ld), so
//	.text + .rodata together occupy a prefix of entries 0..255, capped at
//	16M by the length of the DROM linker region. PSRAM lives in the upper
//	half of the DBUS window (0x3D000000, entries 256-511), disjoint from
//	all flash entries.
//
//	The IROM/DROM boundary is variable, since it follows the size of
//	.text. The PSRAM window sits above it either way: the boundary can
//	only move down, never above 0x3D000000, so the vaddr the drivers pass
//	to romCacheDbusMMUSet always stays inside whatever DROM range
//	Cache_Set_IDROM_MMU_Size left behind.
//
//	DCache (data cache) is the L1/L2 cache in front of the DBUS window.
//	It is disabled (ROM Cache_Disable_DCache, which also invalidates all
//	tag memory) while MMU entries are modified and re-enabled after, so
//	that no stale cache lines survive the remap.
package runtime

import "unsafe"

//go:extern _spsram
var _spsram [0]byte

//go:extern _epsram
var _epsram [0]byte

//go:extern _psram_start
var _psram_start [0]byte

//go:extern _psram_end
var _psram_end [0]byte

// ROM functions baked into every ESP32-S3 chip (fixed addresses provided
// via PROVIDE() in targets/esp32s3.ld); see esp32s3/rom/cache.h.

//export Cache_Dbus_MMU_Set
func romCacheDbusMMUSet(extRam uint32, vaddr uint32, paddr uint32, psize uint32, num uint32, fixed uint32) int32

// Cache_Disable_DCache / Cache_Enable_DCache: the documented-safe pairing
// around an MMU change (Cache_Disable_DCache also invalidates all DCache
// tag memory). Cache_Suspend_DCache/Resume, the alternative pairing, is
// explicitly documented as unsafe to use while changing the MMU.

//export Cache_Disable_DCache
func romCacheDisableDCache() uint32

//export Cache_Enable_DCache
func romCacheEnableDCache(autoload uint32)

const mmuAccessSpiram = 0x8000 // SOC_MMU_TYPE / SOC_MMU_ACCESS_SPIRAM: routes entry to PSRAM instead of flash.

// psramInitError holds why initPSRAM gave up, to be reported once there is
// somewhere to report it.
//
// initPSRAM runs from the boot path, before machine.InitSerial - so a panic
// raised inside it formats its message into a serial port that does not exist
// yet and the board stops silently. Moving InitSerial earlier is not an option
// either: on the USB-serial targets it needs the interrupt table, which is set
// up after this. So the failure is recorded and reported by checkPSRAMInit,
// which runs immediately after serial comes up.
var psramInitError string

// failPSRAMInit records the first failure and returns.
//
// First only: later register writes against a chip that never answered produce
// further failures that are consequences, not causes, and the first one is the
// diagnosis.
func failPSRAMInit(reason string) {
	if psramInitError == "" {
		psramInitError = reason
	}
}

// checkPSRAMInit panics if the bring-up failed. Called once serial is up.
func checkPSRAMInit() {
	if psramInitError != "" {
		panic(psramInitError)
	}
}

// psramWindow returns the linker-provided PSRAM vaddr window
// (targets/esp32s3.ld) and its capacity in 64KB MMU pages.
func psramWindow() (start, end uintptr, pageCap int) {
	start = uintptr(unsafe.Pointer(&_psram_start))
	end = uintptr(unsafe.Pointer(&_psram_end))
	pageCap = int((end - start) / 65536)
	return
}

// zeroInitPSRAM zero-initializes .psram BSS after PSRAM has been mapped
// into the MMU, verifying with a known pattern first that writes actually
// reach the chip (a hardware or timing issue could otherwise silently
// drop them).
//
// mappedBytes is how much of the window the MMU actually backs, which is the
// detected chip density capped by the window size - not the size of .psram.
// The linker sizes .psram from what the program declares, and a program that
// declares more than the chip holds would otherwise be zeroed past the end of
// the mapping. Those addresses do not fault: they alias back over pages that
// are mapped, so the clear silently destroys the start of the region it just
// finished clearing. Stopping at the mapping keeps the damage to an honest
// out-of-memory condition instead.
func zeroInitPSRAM(mappedBytes uintptr) {
	spsram := uintptr(unsafe.Pointer(&_spsram))
	epsram := uintptr(unsafe.Pointer(&_epsram))
	if epsram <= spsram {
		return
	}
	if limit := spsram + mappedBytes; epsram > limit {
		epsram = limit
	}
	if epsram <= spsram {
		return
	}

	*(*uint32)(unsafe.Pointer(spsram)) = 0x5A5A5A5A
	if *(*uint32)(unsafe.Pointer(spsram)) != 0x5A5A5A5A {
		failPSRAMInit("PSRAM readback test failed: wrote 0x5A5A5A5A, read back something else")
		return
	}
	*(*uint32)(unsafe.Pointer(spsram)) = 0
	if *(*uint32)(unsafe.Pointer(spsram)) != 0 {
		failPSRAMInit("PSRAM readback test failed: wrote 0, read back something else")
		return
	}

	for addr := spsram + 4; addr < epsram; addr += 4 {
		*(*uint32)(unsafe.Pointer(addr)) = 0
	}
}
