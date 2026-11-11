//go:build bcm2710

package machine

import (
	"device/arm64"
	"device/bcm2710"
	"errors"
)

// SDIO host on the Arasan EMMC controller, the Pi Zero 2 W's bus to its Wi-Fi chip. It polls,
// moves data through the DATA register, and gives the CMD52 and CMD53 of the SDIO Simplified
// Specification v3.00 section 5. The register bits are the SD Host Controller Simplified
// Specification v3.00 section 2.2, with BCM2835 ARM Peripherals section 5 for the BCM's layout.
type SDIO struct {
	rca       uint32
	blockSize [8]uint16
	baseClock uint32
	slow      bool
}

// SDIOConfig is the bus clock, and the pin that powers the card on and off.
type SDIOConfig struct {
	Frequency uint32
	PowerPin  Pin
}

var SDIO0 = &SDIO{}

var (
	ErrSDIOTimeout  = errors.New("sdio: timeout")
	ErrSDIOCommand  = errors.New("sdio: command error")
	ErrSDIOData     = errors.New("sdio: data error")
	ErrSDIOResponse = errors.New("sdio: card reported an error")
	ErrSDIONoCard   = errors.New("sdio: no SDIO card answered CMD5")
)

// CONTROL0, CONTROL1 and STATUS bits.
const (
	sdControl0Bus4      = 1 << 1
	sdControl1ClkIntEn  = 1 << 0
	sdControl1ClkStable = 1 << 1
	sdControl1ClkEn     = 1 << 2
	sdControl1Timeout   = 0xe << 16
	sdControl1ResetHost = 1 << 24
	sdControl1ResetCmd  = 1 << 25
	sdControl1ResetData = 1 << 26
	sdStatusCmdInhibit  = 1 << 0
	sdStatusDatInhibit  = 1 << 1
)

// INTERRUPT bits.
const (
	sdIntCmdDone   = 1 << 0
	sdIntDataDone  = 1 << 1
	sdIntWriteRdy  = 1 << 4
	sdIntReadRdy   = 1 << 5
	sdIntError     = 1 << 15
	sdIntErrorMask = 0xffff0000
)

// CMDTM bits.
const (
	sdTMBlockCount = 1 << 1
	sdTMRead       = 1 << 4
	sdTMMulti      = 1 << 5
	sdRespNone     = 0 << 16
	sdResp48       = 2 << 16
	sdResp48Busy   = 3 << 16
	sdCRCCheck     = 1 << 19
	sdIndexCheck   = 1 << 20
	sdIsData       = 1 << 21

	sdR1 = sdResp48 | sdCRCCheck | sdIndexCheck
	sdR4 = sdResp48
)

// Configure starts the controller, powers the card up through cfg.PowerPin and brings
// the SDIO card to the transfer state with a 4 bit bus.
func (s *SDIO) Configure(cfg SDIOConfig) error {
	if cfg.Frequency == 0 {
		cfg.Frequency = 25 * MHz
	}
	// GPIO34 to 39 on ALT3 are the EMMC controller, CLK with no pull and the rest pulled up.
	// See bcm2708-rpi-zero-w.dts, sdio_pins.
	for p := Pin(34); p <= 39; p++ {
		mode := PinConfig{Mode: PinAlt3}
		p.Configure(mode)
		if p == 34 {
			p.setPull(0)
		} else {
			p.setPull(2)
		}
	}

	if cfg.PowerPin != NoPin {
		cfg.PowerPin.Configure(PinConfig{Mode: PinOutput})
		cfg.PowerPin.Low()
		sdDelayUs(20000)
		cfg.PowerPin.High()
		sdDelayUs(150000)
	}

	SetPowerState(PowerSD, true)
	rate, err := ClockRate(ClockEMMC)
	if err != nil || rate == 0 {
		return errors.New("sdio: no EMMC clock rate from the VideoCore")
	}
	s.baseClock = rate

	emmc := bcm2710.EMMC
	s.write(&emmc.CONTROL0, 0)
	s.write(&emmc.CONTROL1, sdControl1ResetHost)
	if !sdWait(func() bool { return emmc.CONTROL1.Get()&sdControl1ResetHost == 0 }, 100000) {
		return ErrSDIOTimeout
	}
	if err := s.setClock(400 * KHz); err != nil {
		return err
	}
	s.write(&emmc.IRPT_EN, 0)
	s.write(&emmc.IRPT_MASK, 0xffffffff)
	s.write(&emmc.INTERRUPT, 0xffffffff)

	// Reset the I/O part (CCCR ABORT RES), then CMD0, as Linux sdio_reset and mmc_go_idle do.
	s.cmd(52, 1<<31|6<<9|1<<3, sdR1)
	s.cmd(0, 0, sdRespNone)

	ocr, err := s.cmd(5, 0, sdR4)
	if err != nil {
		return ErrSDIONoCard
	}
	for i := 0; ; i++ {
		ocr, err = s.cmd(5, ocr&0x00ff8000, sdR4)
		if err == nil && ocr&(1<<31) != 0 {
			break
		}
		if i > 1000 {
			return ErrSDIOTimeout
		}
		sdDelayUs(1000)
	}

	r, err := s.cmd(3, 0, sdR1)
	if err != nil {
		return err
	}
	s.rca = r >> 16
	if _, err := s.cmd(7, s.rca<<16, sdResp48Busy|sdCRCCheck|sdIndexCheck); err != nil {
		return err
	}

	// 4 bit bus: CCCR Bus Interface Control (SDIO spec section 6.9, offset 0x07).
	bic, err := s.Read8(0, 0x07)
	if err != nil {
		return err
	}
	if err := s.Write8(0, 0x07, bic&^3|2); err != nil {
		return err
	}
	s.write(&emmc.CONTROL0, emmc.CONTROL0.Get()|sdControl0Bus4)
	return s.setClock(cfg.Frequency)
}

// SetBlockSize sets a function's block size in its FBR (SDIO spec section 6.10), or the CCCR's for function 0.
func (s *SDIO) SetBlockSize(fn uint8, size uint16) error {
	base := uint32(fn) * 0x100
	if err := s.Write8(0, base+0x10, byte(size)); err != nil {
		return err
	}
	if err := s.Write8(0, base+0x11, byte(size>>8)); err != nil {
		return err
	}
	s.blockSize[fn&7] = size
	return nil
}

// EnableFunction sets a function's IOEx bit and waits for its IORx bit (CCCR 0x02 and 0x03).
func (s *SDIO) EnableFunction(fn uint8) error {
	en, err := s.Read8(0, 0x02)
	if err != nil {
		return err
	}
	if err := s.Write8(0, 0x02, en|1<<fn); err != nil {
		return err
	}
	for i := 0; i < 1000; i++ {
		rdy, err := s.Read8(0, 0x03)
		if err == nil && rdy&(1<<fn) != 0 {
			return nil
		}
		sdDelayUs(1000)
	}
	return ErrSDIOTimeout
}

// Read8 reads a register with CMD52.
func (s *SDIO) Read8(fn uint8, addr uint32) (byte, error) {
	r, err := s.cmd(52, uint32(fn&7)<<28|(addr&0x1ffff)<<9, sdR1)
	if err != nil {
		return 0, err
	}
	if r&0xcb00 != 0 {
		return 0, ErrSDIOResponse
	}
	return byte(r), nil
}

// Write8 writes a register with CMD52.
func (s *SDIO) Write8(fn uint8, addr uint32, v byte) error {
	r, err := s.cmd(52, 1<<31|uint32(fn&7)<<28|(addr&0x1ffff)<<9|uint32(v), sdR1)
	if err != nil {
		return err
	}
	if r&0xcb00 != 0 {
		return ErrSDIOResponse
	}
	return nil
}

// ReadData reads buf with CMD53. incr steps the address, else it's a FIFO.
// It uses blocks of the function's size for as much as fits, then bytes.
func (s *SDIO) ReadData(fn uint8, addr uint32, buf []byte, incr bool) error {
	return s.transfer(false, fn, addr, buf, incr)
}

// WriteData writes buf with CMD53, as ReadData reads.
func (s *SDIO) WriteData(fn uint8, addr uint32, buf []byte, incr bool) error {
	return s.transfer(true, fn, addr, buf, incr)
}

func (s *SDIO) transfer(write bool, fn uint8, addr uint32, buf []byte, incr bool) error {
	bs := int(s.blockSize[fn&7])
	for len(buf) > 0 {
		n := len(buf)
		block := bs > 0 && n >= bs
		count := n
		size := n
		if block {
			count = n / bs
			if count > 511 {
				count = 511
			}
			n = count * bs
			size = bs
		} else if bs > 0 && n > bs {
			n, count, size = bs, bs, bs
		} else if n > 512 {
			n, count, size = 512, 512, 512
		}
		if err := s.cmd53(write, fn, addr, buf[:n], block, count, size, incr); err != nil {
			return err
		}
		buf = buf[n:]
		if incr {
			addr += uint32(n)
		}
	}
	return nil
}

func (s *SDIO) cmd53(write bool, fn uint8, addr uint32, buf []byte, block bool, count, size int, incr bool) error {
	arg := uint32(fn&7)<<28 | (addr&0x1ffff)<<9 | uint32(count)&0x1ff
	if write {
		arg |= 1 << 31
	}
	if block {
		arg |= 1 << 27
	}
	if incr {
		arg |= 1 << 26
	}
	blocks := 1
	flags := uint32(sdR1 | sdIsData)
	if block {
		blocks = count
		flags |= sdTMBlockCount | sdTMMulti
	}
	if !write {
		flags |= sdTMRead
	}
	emmc := bcm2710.EMMC
	if !sdWait(func() bool { return emmc.STATUS.Get()&(sdStatusCmdInhibit|sdStatusDatInhibit) == 0 }, 100000) {
		return ErrSDIOTimeout
	}
	s.write(&emmc.BLKSIZECNT, uint32(blocks)<<16|uint32(size))
	r, err := s.issue(53, arg, flags)
	if err != nil {
		return err
	}
	if r&0xcb00 != 0 {
		return ErrSDIOResponse
	}

	ready := uint32(sdIntReadRdy)
	if write {
		ready = sdIntWriteRdy
	}
	for b := 0; b < blocks; b++ {
		if err := s.waitInt(ready); err != nil {
			return err
		}
		s.write(&emmc.INTERRUPT, ready)
		chunk := buf[b*size:]
		if len(chunk) > size {
			chunk = chunk[:size]
		}
		for i := 0; i < len(chunk); i += 4 {
			if write {
				var w uint32
				for j := 0; j < 4 && i+j < len(chunk); j++ {
					w |= uint32(chunk[i+j]) << (8 * j)
				}
				emmc.DATA.Set(w)
			} else {
				w := emmc.DATA.Get()
				for j := 0; j < 4 && i+j < len(chunk); j++ {
					chunk[i+j] = byte(w >> (8 * j))
				}
			}
		}
	}
	if err := s.waitInt(sdIntDataDone); err != nil {
		return err
	}
	s.write(&emmc.INTERRUPT, sdIntDataDone)
	return nil
}

// cmd sends a command with no data and returns RESP0.
func (s *SDIO) cmd(index, arg, flags uint32) (uint32, error) {
	emmc := bcm2710.EMMC
	if !sdWait(func() bool { return emmc.STATUS.Get()&sdStatusCmdInhibit == 0 }, 100000) {
		return 0, ErrSDIOTimeout
	}
	return s.issue(index, arg, flags)
}

func (s *SDIO) issue(index, arg, flags uint32) (uint32, error) {
	emmc := bcm2710.EMMC
	s.write(&emmc.INTERRUPT, 0xffffffff)
	s.write(&emmc.ARG1, arg)
	s.write(&emmc.CMDTM, flags|index<<24)
	if err := s.waitInt(sdIntCmdDone); err != nil {
		s.write(&emmc.CONTROL1, emmc.CONTROL1.Get()|sdControl1ResetCmd|sdControl1ResetData)
		sdWait(func() bool { return emmc.CONTROL1.Get()&(sdControl1ResetCmd|sdControl1ResetData) == 0 }, 100000)
		return 0, err
	}
	s.write(&emmc.INTERRUPT, sdIntCmdDone)
	return emmc.RESP[0].Get(), nil
}

// waitInt waits for an INTERRUPT bit, or returns the error the controller reports.
func (s *SDIO) waitInt(mask uint32) error {
	emmc := bcm2710.EMMC
	for i := 0; i < 1000000; i++ {
		v := emmc.INTERRUPT.Get()
		if v&sdIntErrorMask != 0 || v&sdIntError != 0 {
			s.write(&emmc.INTERRUPT, v&sdIntErrorMask)
			if v&0x000f0000 != 0 {
				return ErrSDIOCommand
			}
			return ErrSDIOData
		}
		if v&mask != 0 {
			return nil
		}
	}
	return ErrSDIOTimeout
}

// setClock divides the base clock as SDHCI v3's 10 bit divider: base / (2 * div).
func (s *SDIO) setClock(hz uint32) error {
	emmc := bcm2710.EMMC
	div := uint32(0)
	if hz < s.baseClock {
		div = (s.baseClock + 2*hz - 1) / (2 * hz)
		if div > 0x3ff {
			div = 0x3ff
		}
	}
	c1 := emmc.CONTROL1.Get() &^ (sdControl1ClkEn | 0xffff)
	s.write(&emmc.CONTROL1, c1)
	sdDelayUs(10)
	c1 |= (div&0xff)<<8 | (div>>8&3)<<6 | sdControl1ClkIntEn | sdControl1Timeout
	s.write(&emmc.CONTROL1, c1)
	if !sdWait(func() bool { return emmc.CONTROL1.Get()&sdControl1ClkStable != 0 }, 100000) {
		return ErrSDIOTimeout
	}
	s.write(&emmc.CONTROL1, c1|sdControl1ClkEn)
	s.slow = hz < 1*MHz
	sdDelayUs(10)
	return nil
}

// write waits after each register write. Two writes within two SD clocks can be lost on
// the BCM2835's controller, as Linux drivers/mmc/host/sdhci-iproc.c says (bcm2835_sdhci_writel).
func (s *SDIO) write(reg interface{ Set(uint32) }, v uint32) {
	reg.Set(v)
	if s.slow {
		sdDelayUs(6)
	} else {
		sdDelayUs(1)
	}
}

func sdWait(done func() bool, limit int) bool {
	for i := 0; i < limit; i++ {
		if done() {
			return true
		}
		sdDelayUs(1)
	}
	return false
}

// sdDelayUs waits on the generic timer, which counts at 19.2 MHz.
func sdDelayUs(us uint32) {
	start := arm64.AsmFull("mrs {}, cntpct_el0", nil)
	ticks := uintptr(us) * 192 / 10
	for arm64.AsmFull("mrs {}, cntpct_el0", nil)-start < ticks {
	}
}
