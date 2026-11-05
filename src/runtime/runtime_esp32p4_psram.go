//go:build esp32p4 && numa_psram_hex

package runtime

import (
	"device/esp"
	"runtime/volatile"
	"unsafe"
)

// This starts the hex PSRAM with an 80 MHz clock and no timing tuning, before the heap, which is
// in PSRAM with this build tag. A full memory test of 32 MB passed at this clock on chip revision 1.3.
// See ESP-IDF v5.3.2 esp_psram/device/esp_psram_impl_ap_hex.c and soc/esp32p4/include/soc/spi_mem_s_reg.h.
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

	psramReadLatency  = 2
	psramWriteLatency = 2
	psramReadDummy    = 2 * (10 - 1)
	psramWriteDummy   = 2 * (5 - 1)

	psramBusDivider = 5 // 400 MHz MPLL to 80 MHz
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
	psramAddrWord = addr
	psramDataWord = 0
	psramROMSetOpMode(psramSPINum, psramModeOPIDTR)
	psramCmd = psramROMCmd{cmd: op, cmdBitLen: 16, addr: &psramAddrWord, addrBitLen: 32, tx: tx, txBitLen: txBits, dummyBitLen: dummy}
	if rxBits > 0 {
		psramCmd.rx = &psramDataWord
		psramCmd.rxBitLen = rxBits
	}
	psramROMCmdConfig(psramSPINum, &psramCmd)
	psramROMCmdStart(psramSPINum, (*byte)(unsafe.Pointer(&psramDataWord)), uint16(rxBits/8), psramCSMask, false)
	return psramDataWord
}

func psramReadMode(addr, bits uint32) uint32 {
	return psramTransfer(psramCmdRegRead, addr, psramReadDummy, nil, 0, bits)
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

	const div = psramBusDivider
	bits := uint32((div-1)<<16 | (div/2-1)<<8 | (div - 1))
	psramReg(psramSramClk).Set(bits)
	psramReg(psramSPI1Clock).Set(bits)
	psramSet(psramSmemTiming, 5)
	psramSet(psramTiming, 5)

	size := setPSRAMModeRegisters()
	if size == 0 {
		println("no hex PSRAM found")
		abort()
	}
	configurePSRAMController()
	for i := uint32(0); i < size/psramMMUPage; i++ {
		psramReg(psramMMUIndex).Set(i)
		// SOC_MMU_PSRAM_VALID and SOC_MMU_ACCESS_PSRAM, from soc/ext_mem_defs.h.
		psramReg(psramMMUContent).Set(i | 1<<11 | 1<<10)
	}
}

func setPSRAMModeRegisters() uint32 {
	v := psramReadMode(0, 16)
	mr0, mr1 := v&0xff, v>>8&0xff
	v = psramReadMode(4, 16)
	mr4, mr8 := v&0xff, v>>8&0xff

	mr0 = mr0&^0x3f | 1<<5 | psramReadLatency<<2
	mr4 = mr4&^0xe0 | psramWriteLatency<<5
	psramWriteMode(0, mr0|mr1<<8)
	psramWriteMode(4, mr4|mr8<<8)

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

// raiseClock runs the CPU and buses from the CPU PLL over 4. This measures 80 MHz on chip revision 1.3.
// The buses on the crystal are too slow for the MIPI DSI bridge to read a frame at 60 MHz.
// See ESP-IDF esp_hw_support/port/esp32p4/rtc_clk.c, rtc_clk_cpu_freq_to_cpll_mhz.
func raiseClock() {
	// The SVD merges the PLLs of these fields. See soc/esp32p4/include/soc/pmu_reg.h, and
	// hal/esp32p4/include/hal/clk_tree_ll.h for the order.
	const xpdCPLL, xpdCPLLI2C, globalCPLLICG = 1 << 27, 1 << 23, 1 << 17
	esp.PMU.IMM_HP_CK_POWER.Set(xpdCPLL | xpdCPLLI2C)
	esp.PMU.IMM_HP_CK_POWER.Set(globalCPLLICG)
	for end := ticks() + nanosecondsToTicks(1e6); ticks() < end; {
	}

	clk := esp.HP_SYS_CLKRST
	update := func() {
		clk.SetROOT_CLK_CTRL0_REG_SOC_CLK_DIV_UPDATE(1)
		for clk.GetROOT_CLK_CTRL0_REG_SOC_CLK_DIV_UPDATE() != 0 {
		}
	}
	clk.SetROOT_CLK_CTRL2_REG_APB_CLK_DIV_NUM(0)
	update()
	clk.SetROOT_CLK_CTRL1_REG_SYS_CLK_DIV_NUM(0)
	update()
	clk.SetROOT_CLK_CTRL1_REG_MEM_CLK_DIV_NUM(0)
	update()
	clk.SetROOT_CLK_CTRL0_REG_CPU_CLK_DIV_NUM(4 - 1)
	update()
	esp.LP_AON_CLKRST.SetLP_AONCLKRST_HP_CLK_CTRL_LP_AONCLKRST_HP_ROOT_CLK_SRC_SEL(1)
}
