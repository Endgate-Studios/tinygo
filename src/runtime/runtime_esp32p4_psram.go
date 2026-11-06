//go:build esp32p4 && numa_psram_hex

package runtime

import (
	"device/esp"
	"runtime/volatile"
	"unsafe"
)

// This starts the hex PSRAM before the heap, which is in PSRAM with this build tag: at 200 MHz, with its timing tuned as
// ESP-IDF tunes it, and at 80 MHz untuned if that fails, where a full memory test of 32 MB passed on chip revision 1.3.
// See ESP-IDF v5.3.2 esp_psram/device/esp_psram_impl_ap_hex.c, esp_hw_support/mspi_timing_tuning.c and
// mspi_timing_by_dqs.c, and soc/esp32p4/include/soc/spi_mem_s_reg.h and iomux_mspi_pin_reg.h.
const (
	psramMSPI0 = 0x5008E000
	psramMSPI1 = 0x5008F000
	psramIOMUX = 0x500E1200

	psramCtrl1      = psramMSPI0 + 0x0c
	psramCacheFctrl = psramMSPI0 + 0x3c
	psramCacheSctrl = psramMSPI0 + 0x40
	psramSramCmd    = psramMSPI0 + 0x44
	psramSramDrd    = psramMSPI0 + 0x48
	psramSramDwr    = psramMSPI0 + 0x4c
	psramSramClk    = psramMSPI0 + 0x50
	psramSmemDdr    = psramMSPI0 + 0xd8
	psramTiming     = psramMSPI0 + 0x180
	psramSmemTiming = psramMSPI0 + 0x190
	psramSmemAc     = psramMSPI0 + 0x1a0
	psramMMUContent = psramMSPI0 + 0x37c
	psramMMUIndex   = psramMSPI0 + 0x380
	psramSPI1Clock  = psramMSPI1 + 0x14
	psramSPI1Ddr    = psramMSPI1 + 0xd4

	psramCmdSyncRead  = 0x0000
	psramCmdSyncWrite = 0x8080
	psramCmdRegRead   = 0x4040
	psramCmdRegWrite  = 0xc0c0

	// The chip's latencies, as its mode registers have them, and the controller's dummy cycles to match, for 200 MHz. They
	// work at a slower clock too, only with more of the time spent waiting. A chip starts at a read latency of 2.
	psramReadLatency  = 4 // 14 cycles
	psramWriteLatency = 1 // 7 cycles
	psramReadDummy    = 2 * (14 - 1)
	psramWriteDummy   = 2 * (7 - 1)
	psramResetDummy   = 2 * (10 - 1) // a read before the latency's set

	// Dividers of the 400 MHz MPLL.
	psramDivTuned   = 2    // 200 MHz, once the timing's tuned
	psramDivSafe    = 5    // 80 MHz, which needs no tuning
	psramDivSetup   = 20   // 20 MHz, for setting the chip up and writing what tuning reads back
	psramTestAddr   = 0x80 // where tuning's pattern goes, as ESP-IDF's MSPI_TIMING_PSRAM_TEST_DATA_ADDR
	psramTestWords  = 32
	psramFIFOBytes  = 64
	psramSPINum     = 3
	psramModeOPIDTR = 7
	psramCSMask     = 1 << 1
	psramVendorAP   = 0x0d
	psramMMUPage    = 64 << 10
)

// psramROMCmd is esp_rom_spi_cmd_t, from esp_rom/include/esp32p4/rom/opi_flash.h.
type psramROMCmd struct {
	cmd, cmdBitLen uint16
	addr           *uint32
	addrBitLen     uint32
	tx             *uint32
	txBitLen       uint32
	rx             *uint32
	rxBitLen       uint32
	dummyBitLen    uint32
}

//export esp_rom_spi_cmd_config
func psramROMCmdConfig(spi int32, c *psramROMCmd)

//export esp_rom_spi_cmd_start
func psramROMCmdStart(spi int32, rx *byte, rxLen uint16, cs uint8, isWriteErase bool)

//export esp_rom_spi_set_op_mode
func psramROMSetOpMode(spi int32, mode int32)

var (
	psramCmd      psramROMCmd
	psramAddrWord uint32
	psramDataWord uint32
	psramTxWord   uint32
)

func psramReg(a uintptr) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(a))
}

func psramField(a uintptr, shift, width, v uint32) {
	mask := uint32(1<<width-1) << shift
	r := psramReg(a)
	r.Set(r.Get()&^mask | v<<shift)
}

func psramSet(a uintptr, shift uint32) {
	psramReg(a).SetBits(1 << shift)
}

func psramTransfer(op uint16, addr, dummy uint32, tx *uint32, txBits, rxBits uint32) uint32 {
	return psramTransferTo(op, addr, dummy, tx, txBits, &psramDataWord, rxBits)
}

// psramTransferTo is a transaction on SPI1, to the chip directly, with what's read going to rx.
func psramTransferTo(op uint16, addr, dummy uint32, tx *uint32, txBits uint32, rx *uint32, rxBits uint32) uint32 {
	psramAddrWord = addr
	*rx = 0
	psramROMSetOpMode(psramSPINum, psramModeOPIDTR)
	psramCmd = psramROMCmd{cmd: op, cmdBitLen: 16, addr: &psramAddrWord, addrBitLen: 32, tx: tx, txBitLen: txBits, dummyBitLen: dummy}
	if rxBits > 0 {
		psramCmd.rx = rx
		psramCmd.rxBitLen = rxBits
	}
	psramROMCmdConfig(psramSPINum, &psramCmd)
	psramROMCmdStart(psramSPINum, (*byte)(unsafe.Pointer(rx)), uint16(rxBits/8), psramCSMask, false)
	return *rx
}

// psramRegDummy is the dummy cycles a register read takes: it's the read latency, which is the chip's own until it's set.
var psramRegDummy uint32 = psramResetDummy

func psramReadMode(addr, bits uint32) uint32 {
	return psramTransfer(psramCmdRegRead, addr, psramRegDummy, nil, 0, bits)
}

func psramWriteMode(addr, v uint32) {
	psramTxWord = v
	psramTransfer(psramCmdRegWrite, addr, 0, &psramTxWord, 16, 0)
}

func initPSRAM() {
	raiseClock()
	enablePSRAMSupply()

	esp.PMU.SetRF_PWC_MSPI_PHY_XPD(1)
	for end := ticks() + nanosecondsToTicks(20e6); ticks() < end; {
	}

	clk := esp.HP_SYS_CLKRST
	clk.SetSOC_CLK_CTRL0_REG_PSRAM_SYS_CLK_EN(1)
	clk.SetPERI_CLK_CTRL00_REG_PSRAM_PLL_CLK_EN(1)
	clk.SetHP_RST_EN0_REG_RST_EN_DUAL_MSPI_AXI(1)
	clk.SetHP_RST_EN0_REG_RST_EN_DUAL_MSPI_AXI(0)
	clk.SetHP_RST_EN0_REG_RST_EN_DUAL_MSPI_APB(1)
	clk.SetHP_RST_EN0_REG_RST_EN_DUAL_MSPI_APB(0)
	clk.SetPERI_CLK_CTRL00_REG_PSRAM_CLK_SRC_SEL(1)
	clk.SetPERI_CLK_CTRL00_REG_PSRAM_CORE_CLK_EN(1)

	// Drive strength 2, and power for the DQS pads. See hal/esp32p4/include/hal/mspi_timing_tuning_ll.h.
	for off := uintptr(0x1c); off <= 0x38; off += 4 {
		psramField(psramIOMUX+off, 12, 2, 2)
	}
	for off := uintptr(0x40); off <= 0x64; off += 4 {
		psramField(psramIOMUX+off, 12, 2, 2)
	}
	psramField(psramIOMUX+0x3c, 15, 2, 2)
	psramField(psramIOMUX+0x68, 15, 2, 2)
	psramSet(psramIOMUX+0x3c, 0)
	psramSet(psramIOMUX+0x68, 0)

	psramSet(psramSmemAc, 0)
	psramField(psramSmemAc, 2, 5, 4-1)
	psramSet(psramSmemAc, 1)
	psramField(psramSmemAc, 7, 5, 4-1)
	psramField(psramSmemAc, 25, 6, 3-1)

	// The core clock is the MPLL's 400 MHz itself, and the bus is divided from that.
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL00_REG_PSRAM_CORE_CLK_DIV_NUM(0)
	setPSRAMClock(psramDivSetup)
	psramSet(psramSmemTiming, 5)
	psramSet(psramTiming, 5)

	size := setPSRAMModeRegisters()
	if size == 0 {
		println("no hex PSRAM found")
		abort()
	}
	configurePSRAMController()
	tunePSRAM()
	for i := uint32(0); i < size/psramMMUPage; i++ {
		psramReg(psramMMUIndex).Set(i)
		// SOC_MMU_PSRAM_VALID and SOC_MMU_ACCESS_PSRAM, from soc/ext_mem_defs.h.
		psramReg(psramMMUContent).Set(i | 1<<11 | 1<<10)
	}
	if psramTuned.MHz == 200 && !psramMemoryTest(size) {
		// Tuning read its pattern right, and the memory didn't: the slower clock, which needs no tuning.
		psramTuned = PSRAMTuning{MHz: 80, TestFailed: true}
		clearPSRAMTuning()
		setPSRAMClock(psramDivSafe)
	}
	memzero(unsafe.Pointer(&_psram_start), uintptr(unsafe.Pointer(&_psram_end))-uintptr(unsafe.Pointer(&_psram_start)))
}

// setPSRAMClock divides the 400 MHz MPLL for the bus of both controllers: SPI0, the cache's, and SPI1, which reaches the chip
// directly. See psram_ctrlr_ll_set_bus_clock.
func setPSRAMClock(div uint32) {
	bits := uint32((div-1)<<16 | (div/2-1)<<8 | (div - 1))
	psramReg(psramSramClk).Set(bits)
	psramReg(psramSPI1Clock).Set(bits)
}

// PSRAMTuning is how the PSRAM was started: its clock, and the DQS phase and delay line tuning chose, for the firmware to say.
type PSRAMTuning struct {
	MHz        uint32
	Phase      uint8 // of MSPI_LL_DQS_PHASE_67_5 and on, as a number from 0 to 3
	Delay      uint8 // 0 to 15, on the DQS lines or, from 16, on the data lines
	Passing    uint8 // how many delay lines in a row read the pattern right, of 32
	TestFailed bool  // tuning chose a delay, and a test of the memory failed at it
}

var psramTuned PSRAMTuning

// PSRAMState is what the PSRAM was started with.
func PSRAMState() PSRAMTuning { return psramTuned }

// The pattern that tuning writes and reads back: s_test_data in ESP-IDF's mspi_timing_by_dqs.c.
var psramTestData = [psramTestWords]uint32{
	0x7f786655, 0xa5ff005a, 0x3f3c33aa, 0xa5ff5a00, 0x1f1e9955, 0xa5005aff, 0x0f0fccaa, 0xa55a00ff,
	0x07876655, 0xffa55a00, 0x03c333aa, 0xff00a55a, 0x01e19955, 0xff005aa5, 0x00f0ccaa, 0xff5a00a5,
	0x80786655, 0x00a5ff5a, 0xc03c33aa, 0x00a55aff, 0xe01e9355, 0x00ff5aa5, 0xf00fccaa, 0x005affa5,
	0xf8876655, 0x5aa5ff00, 0xfcc333aa, 0x5affa500, 0xfee19955, 0x5a00a5ff, 0x11f0ccaa, 0x5a00ffa5,
}

var psramTestRead [psramTestWords]uint32

// psramPatternReads says whether the pattern reads back right, by SPI1, in pieces the size of its FIFO.
func psramPatternReads() bool {
	const words = psramFIFOBytes / 4
	for i := 0; i < psramTestWords; i += words {
		psramTransferTo(psramCmdSyncRead, psramTestAddr+uint32(i*4), psramReadDummy, nil, 0, &psramTestRead[i], psramFIFOBytes*8)
	}
	return psramTestRead == psramTestData
}

// tunePSRAM is mspi_timing_psram_tuning with DQS: the pattern written slowly, then at 200 MHz each of four DQS phases read
// once, the middle of those that read right taken, then each of 32 delay lines read 100 times, and the middle of the longest
// run that always read right taken. Nothing that reads right is 80 MHz, untuned.
func tunePSRAM() {
	const words = psramFIFOBytes / 4
	clearPSRAMTuning()
	setPSRAMClock(psramDivSetup)
	psramReg(psramSPI1Ddr).ClearBits(1 << 1) // SPI1's variable dummy, off while tuning
	for i := 0; i < psramTestWords; i += words {
		psramTransferTo(psramCmdSyncWrite, psramTestAddr+uint32(i*4), psramWriteDummy, &psramTestData[i], psramFIFOBytes*8, &psramDataWord, 0)
	}
	setPSRAMClock(psramDivTuned)

	var phaseOK [4]bool
	for phase := range phaseOK {
		setDQSPhase(uint32(phase))
		phaseOK[phase] = psramPatternReads()
	}
	length, end := longestRun(phaseOK[:])
	best := 0
	switch {
	case length == 0:
		psramTuned = PSRAMTuning{MHz: 80}
		clearPSRAMTuning()
		setPSRAMClock(psramDivSafe)
		psramSet(psramSPI1Ddr, 1)
		return
	case length == 1:
		best = end
	case length <= 3:
		best = end - 1
	default:
		best = end - 2
	}
	setDQSPhase(uint32(best))

	var delayOK [32]bool
	for d := range delayOK {
		setDelayLine(d)
		ok := true
		for range 100 {
			if !psramPatternReads() {
				ok = false
				break
			}
		}
		delayOK[d] = ok
	}
	length, end = longestRun(delayOK[:])
	if length <= 1 {
		psramTuned = PSRAMTuning{MHz: 80}
		clearPSRAMTuning()
		setPSRAMClock(psramDivSafe)
		psramSet(psramSPI1Ddr, 1)
		return
	}
	pick := end - length/2
	setDelayLine(pick)
	psramSet(psramSPI1Ddr, 1)
	psramTuned = PSRAMTuning{MHz: 200, Phase: uint8(best), Delay: uint8(pick), Passing: uint8(length)}
}

// longestRun is the length of the longest run of true, and the index it ends at.
func longestRun(ok []bool) (length, end int) {
	run := 0
	for i, good := range ok {
		if good {
			run++
			if run > length {
				length, end = run, i
			}
		} else {
			run = 0
		}
	}
	return length, end
}

// The PSRAM pins' tuning, in IOMUX_MSPI_PIN: data pins with a delay line in bits 4 to 7, and the two DQS pins with a phase in
// bits 1 and 2 and delay lines for each edge in bits 7 to 10 and 17 to 20.
const (
	psramDQS0 = psramIOMUX + 0x3c
	psramDQS1 = psramIOMUX + 0x68
)

func setDQSPhase(phase uint32) {
	psramField(psramDQS0, 1, 2, phase)
	psramField(psramDQS1, 1, 2, phase)
}

// setDelayLine is one of s_test_delayline_config's: 0 to 15 delay the DQS lines by 15 down to 0, and 16 to 31 the data lines by
// 0 up to 15.
func setDelayLine(i int) {
	dqs, data := uint32(0), uint32(0)
	if i < 16 {
		dqs = uint32(15 - i)
	} else {
		data = uint32(i - 16)
	}
	for _, dq := range [...]uintptr{psramDQS0, psramDQS1} {
		psramField(dq, 7, 4, dqs)
		psramField(dq, 17, 4, dqs)
	}
	for off := uintptr(0x1c); off <= 0x38; off += 4 {
		psramField(psramIOMUX+off, 4, 4, data)
	}
	for off := uintptr(0x40); off <= 0x64; off += 4 {
		psramField(psramIOMUX+off, 4, 4, data)
	}
}

// clearPSRAMTuning is mspi_timing_psram_config_clear_tuning_regs: phase and every delay line back to 0.
func clearPSRAMTuning() {
	setDQSPhase(0)
	setDelayLine(15) // DQS and data both 0
}

// psramMemoryTest writes and reads back the top 4 MB of the chip, which nothing uses this early, through the cache: more than it
// holds, so it's the chip that's read. A word's own address, then its inverse, catch both stuck and swapped bits.
func psramMemoryTest(size uint32) bool {
	const testBytes = 4 << 20
	base := uintptr(0x48000000) + uintptr(size) - testBytes
	words := unsafe.Slice((*uint32)(unsafe.Pointer(base)), testBytes/4)
	for pass := uint32(0); pass < 2; pass++ {
		for i := range words {
			v := uint32(base) + uint32(i*4)
			if pass == 1 {
				v = ^v
			}
			words[i] = v
		}
		for i := range words {
			v := uint32(base) + uint32(i*4)
			if pass == 1 {
				v = ^v
			}
			if words[i] != v {
				return false
			}
		}
	}
	return true
}

//go:extern _psram_start
var _psram_start [0]byte

//go:extern _psram_end
var _psram_end [0]byte

func setPSRAMModeRegisters() uint32 {
	v := psramReadMode(0, 16)
	mr0, mr1 := v&0xff, v>>8&0xff
	v = psramReadMode(4, 16)
	mr4, mr8 := v&0xff, v>>8&0xff

	mr0 = mr0&^0x3f | 1<<5 | psramReadLatency<<2
	mr4 = mr4&^0xe0 | psramWriteLatency<<5
	psramWriteMode(0, mr0|mr1<<8)
	psramWriteMode(4, mr4|mr8<<8)
	psramRegDummy = psramReadDummy

	// Burst of 2048 bytes in X16 mode.
	mr8 = psramReadMode(8, 8)&0xff&^0x4f | 3 | 1<<3 | 1<<6
	psramWriteMode(8, mr8)

	if psramReadMode(0, 16)>>8&0x1f != psramVendorAP {
		return 0
	}
	switch psramReadMode(2, 16) & 0x7 {
	case 1:
		return 4 << 20
	case 3:
		return 8 << 20
	case 5:
		return 16 << 20
	case 7:
		return 32 << 20
	case 6:
		return 64 << 20
	}
	return 0
}

func configurePSRAMController() {
	psramSet(psramCacheSctrl, 20)
	psramField(psramSramDwr, 28, 4, 16-1)
	psramField(psramSramDwr, 0, 16, psramCmdSyncWrite)
	psramSet(psramCacheSctrl, 5)
	psramField(psramSramDrd, 28, 4, 16-1)
	psramField(psramSramDrd, 0, 16, psramCmdSyncRead)

	psramField(psramCacheSctrl, 14, 6, 32-1)
	psramSet(psramCacheSctrl, 0)

	psramSet(psramCacheSctrl, 3)
	psramField(psramCacheSctrl, 22, 6, psramWriteDummy-1)
	psramSet(psramCacheSctrl, 4)
	psramField(psramCacheSctrl, 6, 6, psramReadDummy-1)
	psramSet(psramSmemDdr, 1)
	psramSet(psramSPI1Ddr, 1)
	psramSet(psramSramCmd, 23)
	psramSet(psramSmemDdr, 0)

	psramSet(psramCacheSctrl, 21)
	for _, b := range [...]uint32{18, 19, 20, 21, 26, 27} {
		psramSet(psramSramCmd, b)
	}

	psramSet(psramCacheFctrl, 0)
	psramReg(psramCacheFctrl).ClearBits(1 << 31)
	psramSet(psramCtrl1, 26)
	psramSet(psramCtrl1, 25)
}

// enablePSRAMSupply turns on VO2 at 1.9 V: dref 9, mul 4. See hal/esp32p4/include/hal/ldo_ll.h.
func enablePSRAMSupply() {
	esp.PMU.SetEXT_LDO_P1_0P1A_ANA_ANA_0P1A_DREF_1(9)
	esp.PMU.SetEXT_LDO_P1_0P1A_ANA_ANA_0P1A_MUL_1(4)
	esp.PMU.SetEXT_LDO_P1_0P1A__0P1A_TIEH_SEL_1(0)
	esp.PMU.SetEXT_LDO_P1_0P1A__0P1A_TIEH_1(0)
	esp.PMU.SetEXT_LDO_P1_0P1A__0P1A_FORCE_TIEH_SEL_1(1)
	esp.PMU.SetEXT_LDO_P1_0P1A_ANA_ANA_0P1A_EN_VDET_1(1)
	esp.PMU.SetEXT_LDO_P1_0P1A__0P1A_XPD_1(1)
}

// raiseClock runs the CPU at 360 MHz, memory at 180 MHz and APB at 90 MHz. 360 MHz is the most for chips
// below revision 3.0. The core supply values and PLL settings are the ones ESP-IDF v5.3.2 uses.
// See esp_hw_support/port/esp32p4/rtc_clk.c, pmu_param.c, and hal/esp32p4/include/hal/clk_tree_ll.h.
func raiseClock() {
	esp.PMU.SetHP_ACTIVE_BIAS_HP_ACTIVE_DCM_VSET(27)
	esp.PMU.SetHP_ACTIVE_BIAS_HP_ACTIVE_DCM_MODE(1)

	// The SVD merges the PLLs of these fields. See soc/esp32p4/include/soc/pmu_reg.h.
	const xpdCPLL, xpdCPLLI2C, globalCPLLICG = 1 << 27, 1 << 23, 1 << 17
	esp.PMU.IMM_HP_CK_POWER.Set(xpdCPLL | xpdCPLLI2C)
	esp.PMU.IMM_HP_CK_POWER.Set(globalCPLLICG)
	for end := ticks() + nanosecondsToTicks(1e6); ticks() < end; {
	}

	clk := esp.HP_SYS_CLKRST
	clk.SetANA_PLL_CTRL0_REG_CPU_PLL_CAL_STOP(0)
	writeCPLL(2, 5<<4)        // I2C_CPLL_OC_REF_DIV, with dchgp 5
	writeCPLL(3, 9)           // I2C_CPLL_OC_DIV_7_0, for 360 MHz from revision 1.0
	writeCPLL(6, 1<<6|3<<4|3) // I2C_CPLL_OC_DCUR
	for end := ticks() + nanosecondsToTicks(100e6); clk.GetANA_PLL_CTRL0_REG_CPU_PLL_CAL_END() == 0 && ticks() < end; {
	}
	for end := ticks() + nanosecondsToTicks(10e3); ticks() < end; {
	}
	clk.SetANA_PLL_CTRL0_REG_CPU_PLL_CAL_STOP(1)

	update := func() {
		clk.SetROOT_CLK_CTRL0_REG_SOC_CLK_DIV_UPDATE(1)
		for clk.GetROOT_CLK_CTRL0_REG_SOC_CLK_DIV_UPDATE() != 0 {
		}
	}
	clk.SetROOT_CLK_CTRL2_REG_APB_CLK_DIV_NUM(2 - 1)
	update()
	clk.SetROOT_CLK_CTRL1_REG_SYS_CLK_DIV_NUM(0)
	update()
	clk.SetROOT_CLK_CTRL1_REG_MEM_CLK_DIV_NUM(2 - 1)
	update()
	clk.SetROOT_CLK_CTRL0_REG_CPU_CLK_DIV_NUM(0)
	update()
	esp.LP_AON_CLKRST.SetLP_AONCLKRST_HP_CLK_CTRL_LP_AONCLKRST_HP_ROOT_CLK_SRC_SEL(1)
}

// writeCPLL writes a register of the CPU PLL over the analog I2C bus.
// See ESP-IDF components/esp_rom/patches/esp_rom_regi2c_esp32p4.c.
func writeCPLL(reg, data uint32) {
	const (
		anaMaster = 0x50124000
		conf1     = anaMaster + 0x1c
		conf2     = anaMaster + 0x20
		clk160    = anaMaster + 0x34
		cpllBlock = 0x67
		cpllSel   = 1 << 11
		busy      = 1 << 25
		write     = 1 << 24
	)
	psramSet(clk160, 0)
	psramReg(conf2).ClearBits(0xffffff)
	psramReg(conf1).ClearBits(0xffffff)
	psramReg(conf2).SetBits(cpllSel)
	ctrl := psramReg(anaMaster)
	for ctrl.Get()&busy != 0 {
	}
	ctrl.Set(cpllBlock | reg<<8 | write | data<<16)
	for ctrl.Get()&busy != 0 {
	}
}
