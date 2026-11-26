// Package cyw43 drives a CYW43430/43436 Wi-Fi chip on SDIO, as on the Raspberry Pi Zero 2 W.
// It loads the chip's firmware, talks to it with the SDPCM and BCDC protocols of Linux
// drivers/net/wireless/broadcom/brcm80211/brcmfmac, joins a network, and moves Ethernet
// frames. The chip's own supplicant does WPA2.
package cyw43

import (
	"errors"
	"sync"
	"time"
)

// Bus is an SDIO host: CMD52 for one register, CMD53 for data.
type Bus interface {
	Read8(fn uint8, addr uint32) (byte, error)
	Write8(fn uint8, addr uint32, v byte) error
	ReadData(fn uint8, addr uint32, buf []byte, incr bool) error
	WriteData(fn uint8, addr uint32, buf []byte, incr bool) error
	SetBlockSize(fn uint8, size uint16) error
	EnableFunction(fn uint8) error
}

// SDIO functions.
const (
	fnBus       = 0
	fnBackplane = 1
	fnWLAN      = 2
)

// Function 1 registers (brcmfmac sdio.h, SBSDIO_*).
const (
	regWatermark  = 0x10008
	regSBAddrLow  = 0x1000a
	regSBAddrMid  = 0x1000b
	regSBAddrHigh = 0x1000c
	regChipClock  = 0x1000e
	regPullUp     = 0x1000f
	regWakeupCtl  = 0x1001e
	regSleepCSR   = 0x1001f

	clockForceALP     = 0x01
	clockForceHT      = 0x02
	clockALPRequest   = 0x08
	clockHTRequest    = 0x10
	clockForceHWOff   = 0x20
	clockALPAvailable = 0x40
	clockHTAvailable  = 0x80

	sbWindowMask = 0xffff8000
	sbOffsetMask = 0x7fff
	sbAccess4B   = 0x8000
)

// Backplane cores of the 43430 family, as embassy-rs cyw43 and soypat/cyw43439 hard code them.
const (
	chipCommon   = 0x18000000
	coreSDIO     = 0x18002000
	coreARM      = 0x18003000
	coreSOCRAM   = 0x18004000
	wrapperOff   = 0x100000
	aiIOCtrl     = 0x408
	aiResetCtrl  = 0x800
	ioctlClock   = 0x1
	ioctlFGC     = 0x2
	resetCtrlBit = 0x1

	socramBankIndex = coreSOCRAM + 0x10
	socramBankPDA   = coreSOCRAM + 0x44

	ramSize = 0x80000
)

// SDIO device core registers (brcmfmac sdio.c, struct sdpcmd_regs).
const (
	sdIntStatus     = coreSDIO + 0x20
	sdHostIntMask   = coreSDIO + 0x24
	sdToSBMailbox   = coreSDIO + 0x40
	sdToSBMbData    = coreSDIO + 0x48
	sdToHostMbData  = coreSDIO + 0x4c
	intFrameInd     = 1 << 6
	intHostMailbox  = 1 << 7
	intSWMask       = 0xf0
	smbIntAck       = 1 << 1
	sdpcmProtocol   = 4
	smbVersionShift = 16
)

const blockSizeWLAN = 512

var (
	ErrNoChip      = errors.New("cyw43: no answer from the chip")
	ErrClock       = errors.New("cyw43: the chip's clock didn't start")
	ErrNoFirmware  = errors.New("cyw43: no firmware")
	ErrTimeout     = errors.New("cyw43: timeout")
	ErrIoctl       = errors.New("cyw43: the firmware refused a request")
	ErrJoin        = errors.New("cyw43: couldn't join the network")
	ErrNotJoined   = errors.New("cyw43: not joined")
	ErrFrameTooBig = errors.New("cyw43: frame too big")
)

// Device is one chip.
type Device struct {
	bus    Bus
	mu     sync.Mutex
	window uint32
	chip   uint16
	rev    uint8
	mac    [6]byte

	txSeq  uint8
	txMax  uint8
	reqID  uint16
	txBuf  [2048]byte
	rxBuf  [2048]byte
	ctlBuf []byte
	ctlID  uint16
	ctlOK  bool
	ctlErr bool

	rxq     frameQueue
	handler func([]byte)

	events  eventState
	joined  bool
	started bool
}

// New returns a driver for the chip on bus.
func New(bus Bus) *Device {
	return &Device{bus: bus, window: 0xffffffff}
}

// Init brings up the SDIO functions and the backplane clock, and reads the chip id.
func (d *Device) Init() error {
	if err := d.bus.SetBlockSize(fnBackplane, 64); err != nil {
		return err
	}
	if err := d.bus.EnableFunction(fnBackplane); err != nil {
		return err
	}
	if err := d.bus.SetBlockSize(fnWLAN, blockSizeWLAN); err != nil {
		return err
	}

	// ALP clock, as brcmf_sdio_probe_attach.
	d.bus.Write8(fnBackplane, regChipClock, clockForceHWOff|clockALPRequest)
	if !d.waitClock(clockALPAvailable) {
		return ErrClock
	}
	d.bus.Write8(fnBackplane, regChipClock, clockForceHWOff|clockForceALP)
	d.bus.Write8(fnBackplane, regPullUp, 0)

	id, err := d.read32(chipCommon)
	if err != nil {
		return ErrNoChip
	}
	d.chip = uint16(id)
	d.rev = uint8(id>>16) & 0xf
	return nil
}

// Chip returns the chip id and revision that Init read, 0xa9a6 for the 43430 family.
func (d *Device) Chip() (id uint16, rev uint8) {
	return d.chip, d.rev
}

// Start loads firmware, the NVRAM text and the CLM blob, and starts the chip's CPU.
func (d *Device) Start(firmware, nvram, clm []byte) error {
	if len(firmware) == 0 || len(nvram) == 0 {
		return ErrNoFirmware
	}
	// Hold the CPU, reset the RAM and drop the 43430's bank 3 remap, as brcmf_chip_cm3_set_passive.
	d.coreDisable(coreARM)
	d.coreDisable(coreSOCRAM)
	d.coreReset(coreSOCRAM)
	d.write32(socramBankIndex, 3)
	d.write32(socramBankPDA, 0)

	if err := d.writeRAM(0, firmware); err != nil {
		return err
	}
	nv := processNVRAM(nvram)
	if err := d.writeRAM(ramSize-uint32(len(nv)), nv); err != nil {
		return err
	}

	d.write32(sdIntStatus, 0xffffffff)
	d.coreReset(coreARM)
	if !d.waitClockTimeout(clockHTAvailable, 1000) {
		return ErrClock
	}

	// Keep the HT clock and the SDIO core awake, as brcmf_sdio_sr_init and brcmf_sdio_kso_init.
	d.bus.Write8(fnBackplane, regWakeupCtl, 1<<1)
	d.bus.Write8(fnBackplane, regChipClock, clockForceHT)
	d.bus.Write8(fnBackplane, regSleepCSR, 1)

	d.write32(sdHostIntMask, intSWMask)
	d.write32(sdToSBMbData, sdpcmProtocol<<smbVersionShift)
	d.bus.Write8(fnBackplane, regWatermark, 8)
	if err := d.bus.EnableFunction(fnWLAN); err != nil {
		return err
	}
	d.txMax = 1
	d.started = true

	// The firmware is up once it answers.
	var ver [64]byte
	ok := false
	for i := 0; i < 50; i++ {
		if _, err := d.getVar("ver", ver[:]); err == nil {
			ok = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		return ErrTimeout
	}
	if len(clm) > 0 {
		if err := d.loadCLM(clm); err != nil {
			return err
		}
	}
	return d.setup()
}

// Version returns the firmware's version text.
func (d *Device) Version() string {
	var ver [128]byte
	n, err := d.getVar("ver", ver[:])
	if err != nil {
		return ""
	}
	for i := 0; i < n; i++ {
		if ver[i] == 0 {
			return string(ver[:i])
		}
	}
	return string(ver[:n])
}

func (d *Device) waitClock(bit byte) bool {
	return d.waitClockTimeout(bit, 100)
}

func (d *Device) waitClockTimeout(bit byte, ms int) bool {
	for i := 0; i < ms; i++ {
		v, err := d.bus.Read8(fnBackplane, regChipClock)
		if err == nil && v&bit != 0 {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// setWindow points function 1's 32 KB window at the backplane address.
func (d *Device) setWindow(addr uint32) error {
	w := addr & sbWindowMask
	if w == d.window {
		return nil
	}
	if err := d.bus.Write8(fnBackplane, regSBAddrLow, byte(w>>8)); err != nil {
		return err
	}
	if err := d.bus.Write8(fnBackplane, regSBAddrMid, byte(w>>16)); err != nil {
		return err
	}
	if err := d.bus.Write8(fnBackplane, regSBAddrHigh, byte(w>>24)); err != nil {
		return err
	}
	d.window = w
	return nil
}

func (d *Device) read32(addr uint32) (uint32, error) {
	if err := d.setWindow(addr); err != nil {
		return 0, err
	}
	var b [4]byte
	if err := d.bus.ReadData(fnBackplane, addr&sbOffsetMask|sbAccess4B, b[:], true); err != nil {
		return 0, err
	}
	return le32(b[:]), nil
}

func (d *Device) write32(addr, v uint32) error {
	if err := d.setWindow(addr); err != nil {
		return err
	}
	var b [4]byte
	put32(b[:], v)
	return d.bus.WriteData(fnBackplane, addr&sbOffsetMask|sbAccess4B, b[:], true)
}

func (d *Device) read8(addr uint32) byte {
	d.setWindow(addr)
	v, _ := d.bus.Read8(fnBackplane, addr&sbOffsetMask)
	return v
}

func (d *Device) write8(addr uint32, v byte) {
	d.setWindow(addr)
	d.bus.Write8(fnBackplane, addr&sbOffsetMask, v)
}

// writeRAM copies data to the chip's RAM in pieces that stay inside one window.
func (d *Device) writeRAM(addr uint32, data []byte) error {
	var tail [4]byte
	for len(data) > 0 {
		n := int(0x8000 - addr&sbOffsetMask)
		if n > 2048 {
			n = 2048
		}
		if n > len(data) {
			n = len(data)
		}
		chunk := data[:n]
		if n%4 != 0 {
			// The host moves whole words, so the last piece is padded.
			if n > 4 {
				n -= n % 4
				chunk = data[:n]
			} else {
				tail = [4]byte{}
				copy(tail[:], data[:n])
				chunk = tail[:]
			}
		}
		if err := d.setWindow(addr); err != nil {
			return err
		}
		if err := d.bus.WriteData(fnBackplane, addr&sbOffsetMask|sbAccess4B, chunk, true); err != nil {
			return err
		}
		addr += uint32(n)
		data = data[n:]
	}
	return nil
}

// coreDisable and coreReset are embassy-rs cyw43's, on a core's wrapper registers.
func (d *Device) coreDisable(core uint32) {
	w := core + wrapperOff
	d.read8(w + aiResetCtrl)
	if d.read8(w+aiResetCtrl)&resetCtrlBit != 0 {
		return
	}
	d.write8(w+aiIOCtrl, 0)
	d.read8(w + aiIOCtrl)
	time.Sleep(time.Millisecond)
	d.write8(w+aiResetCtrl, resetCtrlBit)
	d.read8(w + aiResetCtrl)
}

func (d *Device) coreReset(core uint32) {
	d.coreDisable(core)
	w := core + wrapperOff
	d.write8(w+aiIOCtrl, ioctlFGC|ioctlClock)
	d.read8(w + aiIOCtrl)
	d.write8(w+aiResetCtrl, 0)
	time.Sleep(time.Millisecond)
	d.write8(w+aiIOCtrl, ioctlClock)
	d.read8(w + aiIOCtrl)
	time.Sleep(time.Millisecond)
}

// processNVRAM turns the NVRAM text into NUL separated lines, padded, with the length token
// last, as brcmf_fw_nvram_strip does.
func processNVRAM(text []byte) []byte {
	out := make([]byte, 0, len(text)+8)
	for len(text) > 0 {
		end := 0
		for end < len(text) && text[end] != '\n' {
			end++
		}
		line := text[:end]
		if end < len(text) {
			text = text[end+1:]
		} else {
			text = nil
		}
		for len(line) > 0 && (line[len(line)-1] == '\r' || line[len(line)-1] == ' ' || line[len(line)-1] == '\t') {
			line = line[:len(line)-1]
		}
		for len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			line = line[1:]
		}
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		out = append(out, line...)
		out = append(out, 0)
	}
	out = append(out, 0)
	for len(out)%4 != 0 {
		out = append(out, 0)
	}
	words := uint32(len(out) / 4)
	var tok [4]byte
	put32(tok[:], (^words)<<16|words&0xffff)
	return append(out, tok[:]...)
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func le16(b []byte) uint16 {
	return uint16(b[0]) | uint16(b[1])<<8
}

func be32(b []byte) uint32 {
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}

func be16(b []byte) uint16 {
	return uint16(b[1]) | uint16(b[0])<<8
}

func put32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

func put16(b []byte, v uint16) {
	b[0], b[1] = byte(v), byte(v>>8)
}
