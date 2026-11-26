//go:build esp32s3 || esp32p4

package machine

// The SD/MMC host of the ESP32-S3 and the ESP32-P4: a Synopsys DesignWare host with two slots, each a memory card or an SDIO
// device, such as an ESP32-C6 running ESP-Hosted. This is the host: it sends commands, sets the clock and bus width, and moves
// data with its own DMA. What a card is told is its driver's.
//
// It polls the host rather than taking its interrupt, and yields to other goroutines while it waits. Both slots can be open at once:
// a command or transfer has the host to itself, and the host's clock is set for the slot it's on.
//
// The register work follows ESP-IDF v5.3.2 (esp_driver_sdmmc/src/sdmmc_host.c and sdmmc_transaction.c, hal/esp32s3 and
// hal/esp32p4/include/hal/sdmmc_ll.h). What differs between the chips, the clocks, the pins and how DMA memory is reached, is in
// machine_esp32s3_sdmmc.go and machine_esp32p4_sdmmc.go.

import (
	"device/esp"
	"sync"
	"unsafe"
)

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrSDMMCTimeout  Error = "sdmmc: no answer"
	ErrSDMMCCRC      Error = "sdmmc: a response's CRC was wrong"
	ErrSDMMCResponse Error = "sdmmc: a response was bad"
	ErrSDMMCDataCRC  Error = "sdmmc: data CRC error"
	ErrSDMMCData     Error = "sdmmc: data transfer failed"
	ErrSDMMCBusy     Error = "sdmmc: the card stayed busy"
	ErrSDMMCHost     Error = "sdmmc: the host didn't take a command"
	ErrSDMMCSlot     Error = "sdmmc: a slot is 0 or 1, and open once"
	ErrSDMMCWidth    Error = "sdmmc: a bus is 1 or 4 data lines, as wired"
	ErrSDMMCMemory   Error = "sdmmc: DMA memory must be internal RAM"
	ErrSDMMCLength   Error = "sdmmc: a transfer is whole blocks of 4 to 512 bytes, 64 KB at most"
)

// SDMMCConfig is a slot's wiring. Pins a narrower bus doesn't use are NoPin.
type SDMMCConfig struct {
	Slot                     int // 0 or 1
	Width                    int // data lines wired: 1 or 4
	CLK, CMD, D0, D1, D2, D3 Pin
}

// SDMMCFlags say what a command expects back.
type SDMMCFlags uint32

const (
	SDMMCNoResponse    SDMMCFlags = 0
	SDMMCResponse      SDMMCFlags = sdmmcRespExpect | sdmmcCheckCRC                 // R1, R5, R6, R7
	SDMMCResponseNoCRC SDMMCFlags = sdmmcRespExpect                                 // R3, R4: their CRC field isn't one
	SDMMCLongResponse  SDMMCFlags = sdmmcRespExpect | sdmmcRespLong | sdmmcCheckCRC // R2
	// SDMMCInit sends the card 80 clocks first, as CMD0 needs.
	SDMMCInit SDMMCFlags = sdmmcSendInit
)

// SDMMCSlot is one open slot.
type SDMMCSlot struct {
	slot    uint32
	cfg     SDMMCConfig
	hostDiv uint32 // the host's clock is 160 MHz / hostDiv
	cardDiv uint32 // and the card's that / (2 * cardDiv), unless it's 0
	width   int
}

// RINTSTS bits (soc/esp32/include/soc/sdmmc_reg.h, the same on the S3 and P4).
const (
	sdmmcIntRespErr  = 1 << 1
	sdmmcIntCmdDone  = 1 << 2
	sdmmcIntDataOver = 1 << 3
	sdmmcIntRCRC     = 1 << 6
	sdmmcIntDCRC     = 1 << 7
	sdmmcIntRTO      = 1 << 8
	sdmmcIntDRTO     = 1 << 9
	sdmmcIntHTO      = 1 << 10
	sdmmcIntFRUN     = 1 << 11
	sdmmcIntHLE      = 1 << 12
	sdmmcIntSBE      = 1 << 13
	sdmmcIntEBE      = 1 << 15

	sdmmcIntDataErr = sdmmcIntDCRC | sdmmcIntDRTO | sdmmcIntHTO | sdmmcIntFRUN | sdmmcIntSBE | sdmmcIntEBE
)

// CMD register bits (sdmmc_hw_cmd_t in soc/esp32s3/include/soc/sdmmc_struct.h).
const (
	sdmmcRespExpect  = 1 << 6
	sdmmcRespLong    = 1 << 7
	sdmmcCheckCRC    = 1 << 8
	sdmmcData        = 1 << 9
	sdmmcWrite       = 1 << 10
	sdmmcAutoStop    = 1 << 12
	sdmmcWaitPrev    = 1 << 13
	sdmmcSendInit    = 1 << 15
	sdmmcCardPos     = 16 // which slot
	sdmmcUpdateClock = 1 << 21
	sdmmcUseHold     = 1 << 29
	sdmmcStart       = 1 << 31
)

// CTRL, BMOD and STATUS bits.
const (
	sdmmcCtrlControllerReset = 1 << 0
	sdmmcCtrlFIFOReset       = 1 << 1
	sdmmcCtrlDMAReset        = 1 << 2
	sdmmcCtrlIntEnable       = 1 << 4
	sdmmcCtrlDMAEnable       = 1 << 5
	sdmmcCtrlUseIDMAC        = 1 << 25

	sdmmcBmodSWReset    = 1 << 0
	sdmmcBmodFixedBurst = 1 << 1
	sdmmcBmodEnable     = 1 << 7

	sdmmcStatusDataBusy = 1 << 9
)

// sdmmcDesc is an IDMAC descriptor (sdmmc_desc_t).
type sdmmcDesc struct {
	ctrl uint32 // DIC bit 1, LD bit 2, FS bit 3, CH bit 4, OWN bit 31
	size uint32 // buffer 1's size, bits 0-12
	buf  uint32
	next uint32
}

const (
	sdmmcDescLast    = 1 << 2
	sdmmcDescFirst   = 1 << 3
	sdmmcDescChained = 1 << 4
	sdmmcDescOwned   = 1 << 31
	sdmmcDescMax     = 4096 // bytes one descriptor moves
	sdmmcMaxDescs    = 16

	// SDMMCMaxTransfer is the most one transfer moves.
	SDMMCMaxTransfer = sdmmcMaxDescs * sdmmcDescMax
)

// sdmmcDescs is the chain: a package variable, so in internal RAM, where the host's DMA reads it.
var sdmmcDescs [sdmmcMaxDescs]sdmmcDesc

var sdmmc struct {
	mu     sync.Mutex
	open   [2]*SDMMCSlot
	active *SDMMCSlot // the slot the host's clock is set for
}

// OpenSDMMC brings up the host, if no slot has, and a slot on it: its pins, one data line, and a clock of 400 kHz, as a card starts.
// A 4-bit slot's D3 is held high until SetWidth(4), which keeps an SD card in SD mode at CMD0.
func OpenSDMMC(cfg SDMMCConfig) (*SDMMCSlot, error) {
	if cfg.Slot != 0 && cfg.Slot != 1 {
		return nil, ErrSDMMCSlot
	}
	if cfg.Width != 1 && cfg.Width != 4 {
		return nil, ErrSDMMCWidth
	}
	sdmmc.mu.Lock()
	defer sdmmc.mu.Unlock()
	if sdmmc.open[cfg.Slot] != nil {
		return nil, ErrSDMMCSlot
	}
	if sdmmc.open[0] == nil && sdmmc.open[1] == nil {
		if err := sdmmcHostOn(); err != nil {
			return nil, err
		}
	}
	s := &SDMMCSlot{slot: uint32(cfg.Slot), cfg: cfg, width: 1}
	sdmmcRoutePins(cfg)
	sdmmc.open[cfg.Slot] = s
	esp.SDHOST.CTYPE.ClearBits(1<<s.slot | 1<<(16+s.slot))
	if err := s.setClock(400); err != nil {
		s.closeLocked()
		return nil, err
	}
	return s, nil
}

// Close lets the slot go, and the host too, if it was the last.
func (s *SDMMCSlot) Close() {
	sdmmc.mu.Lock()
	defer sdmmc.mu.Unlock()
	s.closeLocked()
}

func (s *SDMMCSlot) closeLocked() {
	if sdmmc.open[s.slot] != s {
		return
	}
	esp.SDHOST.CLKENA.ClearBits(1<<s.slot | 1<<(16+s.slot))
	sdmmc.open[s.slot] = nil
	if sdmmc.active == s {
		sdmmc.active = nil
	}
	if sdmmc.open[0] == nil && sdmmc.open[1] == nil {
		sdmmcHostOff()
	}
}

// SetClock sets the card's clock, as near kHz as the host can without going over: 400 kHz to start a card, 20 MHz at an SD card's
// default speed, 40 MHz at high speed, as ESP-IDF's sdmmc_host_set_card_clk has them.
func (s *SDMMCSlot) SetClock(kHz uint32) error {
	sdmmc.mu.Lock()
	defer sdmmc.mu.Unlock()
	return s.setClock(kHz)
}

func (s *SDMMCSlot) setClock(kHz uint32) error {
	switch {
	case kHz <= 400:
		s.hostDiv, s.cardDiv = 10, 20 // 160 MHz / 10 / 40
	case kHz == 20_000:
		s.hostDiv, s.cardDiv = 8, 0
	case kHz == 40_000:
		s.hostDiv, s.cardDiv = 4, 0
	default:
		s.hostDiv, s.cardDiv = 2, (80_000+2*kHz-1)/(2*kHz) // from 80 MHz
	}
	sdmmc.active = nil
	return s.activate()
}

// KHz is the card's clock.
func (s *SDMMCSlot) KHz() uint32 {
	khz := 160_000 / s.hostDiv
	if s.cardDiv > 0 {
		khz /= 2 * s.cardDiv
	}
	return khz
}

// SetWidth sets the bus to 1 or 4 data lines, once the card's been told.
func (s *SDMMCSlot) SetWidth(lines int) error {
	if lines != 1 && lines != 4 || lines > s.cfg.Width {
		return ErrSDMMCWidth
	}
	sdmmc.mu.Lock()
	defer sdmmc.mu.Unlock()
	if lines == 4 {
		esp.SDHOST.CTYPE.SetBits(1 << s.slot)
		if s.width != 4 {
			sdmmcRouteD3(s.cfg)
		}
	} else {
		esp.SDHOST.CTYPE.ClearBits(1 << s.slot)
	}
	s.width = lines
	return nil
}

// Command sends a command and waits for its response, which it gives: the first word, all a short response has.
func (s *SDMMCSlot) Command(index, arg uint32, flags SDMMCFlags) (uint32, error) {
	sdmmc.mu.Lock()
	defer sdmmc.mu.Unlock()
	if err := s.activate(); err != nil {
		return 0, err
	}
	return s.command(index, arg, uint32(flags))
}

// LongResponse is the whole of the last response, for an SDMMCLongResponse: its first word is the low bits.
func (s *SDMMCSlot) LongResponse() [4]uint32 {
	hw := esp.SDHOST
	return [4]uint32{hw.RESP0.Get(), hw.RESP1.Get(), hw.RESP2.Get(), hw.RESP3.Get()}
}

// Transfer sends a command that moves buf by DMA, and waits for it to be done. buf is from SDMMCMemory. It's whole blocks of
// blockSize bytes, or one of up to 512, a multiple of 4, which SDIO's byte mode moves. stop has the host send CMD12 at the end, as a
// memory card's multi-block command needs. After a write to a memory card, WaitNotBusy until it's stored it.
func (s *SDMMCSlot) Transfer(index, arg uint32, buf []byte, blockSize int, write, stop bool) error {
	n := len(buf)
	if n == 0 || n > SDMMCMaxTransfer || n%4 != 0 || blockSize < 4 || blockSize > 512 || n > blockSize && n%blockSize != 0 {
		return ErrSDMMCLength
	}
	if n < blockSize {
		blockSize = n
	}
	sdmmc.mu.Lock()
	defer sdmmc.mu.Unlock()
	if err := s.activate(); err != nil {
		return err
	}
	hw := esp.SDHOST
	// The chain: a descriptor per 4 KB.
	base := sdmmcDMAAddress(unsafe.Pointer(&buf[0]))
	chain := sdmmcDescsForCPU()
	count := (n + sdmmcDescMax - 1) / sdmmcDescMax
	for i := range count {
		d := &chain[i]
		d.ctrl = sdmmcDescChained | sdmmcDescOwned
		if i == 0 {
			d.ctrl |= sdmmcDescFirst
		}
		d.size = uint32(min(n-i*sdmmcDescMax, sdmmcDescMax))
		d.buf = base + uint32(i*sdmmcDescMax)
		d.next = 0
		if i == count-1 {
			d.ctrl |= sdmmcDescLast
		} else {
			d.next = sdmmcDMAAddress(unsafe.Pointer(&sdmmcDescs[i+1]))
		}
	}
	// sdmmc_host_dma_prepare
	hw.BYTCNT.Set(uint32(n))
	hw.BLKSIZ.Set(uint32(blockSize))
	hw.DBADDR.Set(sdmmcDMAAddress(unsafe.Pointer(&sdmmcDescs[0])))
	hw.IDSTS.Set(0x3ff) // the last transfer's DMA status; ESP-IDF's interrupt clears it
	hw.CTRL.SetBits(sdmmcCtrlDMAEnable | sdmmcCtrlUseIDMAC)
	hw.BMOD.SetBits(sdmmcBmodEnable | sdmmcBmodFixedBurst)
	hw.PLDMND.Set(1)

	flags := uint32(SDMMCResponse) | sdmmcData
	if write {
		flags |= sdmmcWrite
	}
	if stop {
		flags |= sdmmcAutoStop
	}
	if _, err := s.command(index, arg, flags); err != nil {
		sdmmcStopDMA()
		return err
	}
	// The data: done when the host says it's over, or an error.
	deadline := nanotime() + 2e9
	for {
		r := hw.RINTSTS.Get()
		if r&sdmmcIntDataErr != 0 {
			hw.RINTSTS.Set(r)
			hw.CTRL.SetBits(sdmmcCtrlFIFOReset)
			sdmmcStopDMA()
			if r&sdmmcIntDCRC != 0 {
				return ErrSDMMCDataCRC
			}
			return ErrSDMMCData
		}
		if r&sdmmcIntDataOver != 0 {
			return nil
		}
		if nanotime() > deadline {
			sdmmcStopDMA()
			return ErrSDMMCTimeout
		}
		gosched()
	}
}

// WaitNotBusy waits while the card holds D0 low, writing to its flash or switching state.
func (s *SDMMCSlot) WaitNotBusy() error {
	deadline := nanotime() + 1e9
	for {
		sdmmc.mu.Lock()
		err := s.activate()
		busy := esp.SDHOST.STATUS.Get()&sdmmcStatusDataBusy != 0
		sdmmc.mu.Unlock()
		if err != nil {
			return err
		}
		if !busy {
			return nil
		}
		if nanotime() > deadline {
			return ErrSDMMCBusy
		}
		gosched()
	}
}

// SDMMCMemory is b as DMA memory: from its first 4-byte boundary, and at most SDMMCMaxTransfer. It's the CPU's view of it, which
// on the P4 is its uncached address, so that what the DMA moves is what's read. b must be internal RAM, best a package's variable.
func SDMMCMemory(b []byte) ([]byte, error) {
	if len(b) > 0 {
		if a := uintptr(unsafe.Pointer(&b[0])) % 4; a != 0 {
			b = b[min(4-int(a), len(b)):]
		}
	}
	b = b[:min(len(b)&^3, SDMMCMaxTransfer)]
	if len(b) == 0 || !sdmmcInternal(unsafe.Pointer(&b[0])) {
		return nil, ErrSDMMCMemory
	}
	return sdmmcForCPU(b), nil
}

// activate sets the host's clock and the slot's, if the host was last on another slot or its clock has changed. sdmmc.mu is held.
func (s *SDMMCSlot) activate() error {
	if sdmmc.active == s {
		return nil
	}
	hw := esp.SDHOST
	hw.CLKENA.ClearBits(1 << s.slot)
	if err := s.updateClock(); err != nil {
		return err
	}
	shift := 8 * s.slot
	hw.CLKDIV.Set(hw.CLKDIV.Get()&^(0xff<<shift) | s.cardDiv<<shift) // the slot's divider is its own
	hw.CLKSRC.Set(hw.CLKSRC.Get()&^(3<<(2*s.slot)) | s.slot<<(2*s.slot))
	sdmmcSetHostDiv(s.hostDiv)
	if err := s.updateClock(); err != nil {
		return err
	}
	// On, and kept on while the card's idle: an SDIO card says it has something on D1, which takes a clock.
	hw.CLKENA.SetBits(1 << s.slot)
	if err := s.updateClock(); err != nil {
		return err
	}
	// Reads time out after 100 ms; responses after the most, 255 clocks.
	hw.TMOUT.Set(min(100*s.KHz(), 0xffffff)<<8 | 255)
	sdmmc.active = s
	return nil
}

// command sends a command on the slot. sdmmc.mu is held, and the slot active.
func (s *SDMMCSlot) command(index, arg, flags uint32) (uint32, error) {
	hw := esp.SDHOST
	if err := sdmmcWaitStart(); err != nil {
		return 0, err
	}
	hw.RINTSTS.Set(0xffffffff)
	if flags&sdmmcSendInit == 0 {
		flags |= sdmmcWaitPrev
	}
	hw.CMDARG.Set(arg)
	hw.CMD.Set(index | flags | s.slot<<sdmmcCardPos | sdmmcUseHold | sdmmcStart)
	deadline := nanotime() + 1e9
	for {
		r := hw.RINTSTS.Get()
		if r&sdmmcIntHLE != 0 {
			hw.RINTSTS.Set(sdmmcIntHLE)
			return 0, ErrSDMMCHost
		}
		if r&sdmmcIntCmdDone != 0 {
			switch {
			case r&sdmmcIntRTO != 0:
				return 0, ErrSDMMCTimeout
			case flags&sdmmcCheckCRC != 0 && r&sdmmcIntRCRC != 0:
				return 0, ErrSDMMCCRC
			case r&sdmmcIntRespErr != 0:
				return 0, ErrSDMMCResponse
			}
			return hw.RESP0.Get(), nil
		}
		if nanotime() > deadline {
			return 0, ErrSDMMCTimeout
		}
		gosched()
	}
}

// updateClock has the host take the clock registers.
func (s *SDMMCSlot) updateClock() error {
	hw := esp.SDHOST
	deadline := nanotime() + 1e9
	for {
		if err := sdmmcWaitStart(); err != nil {
			return err
		}
		hw.CMD.Set(sdmmcUpdateClock | sdmmcWaitPrev | s.slot<<sdmmcCardPos | sdmmcUseHold | sdmmcStart)
		for {
			if nanotime() > deadline {
				return ErrSDMMCHost
			}
			// A clock update can raise HLE: then it's sent again.
			if hw.RINTSTS.Get()&sdmmcIntHLE != 0 {
				hw.RINTSTS.Set(sdmmcIntHLE)
				break
			}
			if hw.CMD.Get()&sdmmcStart == 0 {
				return nil
			}
		}
	}
}

// sdmmcWaitStart waits for the host to take the last command.
func sdmmcWaitStart() error {
	deadline := nanotime() + 1e9
	for esp.SDHOST.CMD.Get()&sdmmcStart != 0 {
		if nanotime() > deadline {
			return ErrSDMMCHost
		}
	}
	return nil
}

// sdmmcReset is the part of sdmmc_host_init that's the same on both chips: the host reset, polled, and its DMA ready.
func sdmmcReset() error {
	hw := esp.SDHOST
	hw.CTRL.SetBits(sdmmcCtrlControllerReset | sdmmcCtrlFIFOReset | sdmmcCtrlDMAReset)
	deadline := nanotime() + 1e9
	for hw.CTRL.Get()&(sdmmcCtrlControllerReset|sdmmcCtrlFIFOReset|sdmmcCtrlDMAReset) != 0 {
		if nanotime() > deadline {
			return ErrSDMMCHost
		}
	}
	hw.RINTSTS.Set(0xffffffff)
	hw.INTMASK.Set(0)                     // polled: RINTSTS is raw, whatever the mask
	hw.CTRL.ClearBits(sdmmcCtrlIntEnable) // and no interrupt
	hw.CARDTHRCTL.ClearBits(1 << 1)       // no busy-clear interrupt
	// sdmmc_host_dma_init
	hw.CTRL.SetBits(sdmmcCtrlDMAEnable)
	hw.BMOD.Set(sdmmcBmodSWReset)
	return nil
}

// sdmmcStopDMA is sdmmc_host_dma_stop, after an error.
func sdmmcStopDMA() {
	hw := esp.SDHOST
	hw.CTRL.ClearBits(sdmmcCtrlUseIDMAC)
	hw.CTRL.SetBits(sdmmcCtrlDMAReset)
	hw.BMOD.ClearBits(sdmmcBmodFixedBurst | sdmmcBmodEnable)
}
