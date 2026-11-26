//go:build esp32p4 && !p4flash

package machine

import "errors"

// This has the API of machine.Flash and machine.RawFlash, and refuses every access. The p4flash tag has
// the real one, in machine_esp32p4_flash_rom.go.

var errFlashNotOnP4 = errors.New("machine: flash storage is not written yet on the ESP32-P4")

const flashSectorSize = 4096

type flashBlockDevice struct{}

// Flash is the flash after the program.
var Flash flashBlockDevice

func (f flashBlockDevice) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, errFlashNotOnP4
}

func (f flashBlockDevice) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, errFlashNotOnP4
}

func (f flashBlockDevice) Size() int64 { return 12 << 20 }

func (f flashBlockDevice) WriteBlockSize() int64 { return 4 }

func (f flashBlockDevice) EraseBlockSize() int64 { return flashSectorSize }

func (f flashBlockDevice) EraseBlocks(start, length int64) error { return errFlashNotOnP4 }

type rawFlash struct{}

// RawFlash is the flash by address on the chip.
var RawFlash rawFlash

func (rawFlash) ReadAt(p []byte, addr uint32) error { return errFlashNotOnP4 }

func (rawFlash) WriteAt(p []byte, addr uint32) error { return errFlashNotOnP4 }

func (rawFlash) EraseSector(addr uint32) error { return errFlashNotOnP4 }
