//go:build esp32p4

package machine

// What the ESP32-P4's SD/MMC host needs of its own: see machine_esp32xx_sdmmc.go. Slot 0's pins are the IO mux's, and slot 1's go
// through the GPIO matrix. Internal RAM is behind a cache, so the CPU reaches what the DMA moves through its uncached address.

import (
	"device/esp"
	"unsafe"
)

// The host's signals (soc/esp32p4/include/soc/gpio_sig_map.h and soc/esp32p4/sdmmc_periph.c): slot 1's through the matrix, and
// both slots' inputs that aren't the bus.
const (
	sdmmcSig1CLK = 0
	sdmmcSig1CMD = 1
	sdmmcSig1D0  = 2 // D1 to D3 follow

	sdmmcMatrixOne  = 0x3F
	sdmmcMatrixZero = 0x3E

	sdmmcIOMuxFunc  = 0 // SDMMC_SLOT0_FUNC in soc/esp32p4/include/soc/sdmmc_pins.h
	sdmmcIOMuxDrive = 3 // as configure_pin_iomux in sdmmc_host.c has

	// sdmmcNonCacheable is SOC_NON_CACHEABLE_OFFSET.
	sdmmcNonCacheable = 0x40000000
)

var (
	sdmmcSigDetect  = [2]uint32{126, 127}
	sdmmcSigCardInt = [2]uint32{128, 129}
	sdmmcSigProtect = [2]uint32{130, 131}
)

// sdmmcHostOn is sdmmc_host_init: the host's clocks and reset, and the host reset itself.
func sdmmcHostOn() error {
	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL1_REG_SDMMC_SYS_CLK_EN(1)
	esp.LP_AON_CLKRST.SetLP_AONCLKRST_HP_SDMMC_EMAC_RST_CTRL_LP_AONCLKRST_RST_EN_SDMMC(1)
	esp.LP_AON_CLKRST.SetLP_AONCLKRST_HP_SDMMC_EMAC_RST_CTRL_LP_AONCLKRST_RST_EN_SDMMC(0)
	esp.HP_SYS_CLKRST.SetREF_CLK_CTRL2_REG_REF_160M_CLK_EN(1)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL01_REG_SDIO_LS_CLK_SRC_SEL(0)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL01_REG_SDIO_LS_CLK_EN(1)
	sdmmcSetHostDiv(2)
	return sdmmcReset()
}

// sdmmcHostOff stops the host's clocks, letting the pins go to another driver.
func sdmmcHostOff() {
	esp.SDHOST.CLK_EDGE_SEL.Set(0)
	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL1_REG_SDMMC_SYS_CLK_EN(0)
}

// sdmmcSetHostDiv is sdmmc_ll_set_clock_div and sdmmc_ll_init_phase_delay, from the PLL's 160 MHz.
func sdmmcSetHostDiv(div uint32) {
	clk := esp.HP_SYS_CLKRST
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_CLK_EDGE_H(div/2 - 1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_CLK_EDGE_N(div - 1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_CLK_EDGE_L(div - 1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_DRV_CLK_EN(1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_SAM_CLK_EN(1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_SLF_CLK_EN(1)
	// 90 degrees on the output clock, for SD's hold time.
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_DRV_CLK_EDGE_SEL(1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_SAM_CLK_EDGE_SEL(0)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_SLF_CLK_EDGE_SEL(0)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_CLK_EDGE_CFG_UPDATE(1)
	clk.SetPERI_CLK_CTRL02_REG_SDIO_LS_CLK_EDGE_CFG_UPDATE(0)
	for end := nanotime() + 10_000; nanotime() < end; {
	}
}

// sdmmcRoutePins gives the slot its pins, with pull-ups, as SD's lines need. A 4-bit slot's D3 is held high as a GPIO, which keeps
// a card in SD mode at CMD0, until sdmmcRouteD3. No card-detect or write-protect pins: the card is there, and writable. The SDIO
// interrupt input is held high.
func sdmmcRoutePins(cfg SDMMCConfig) {
	slot := cfg.Slot
	if slot == 0 {
		sdmmcIOMux(cfg.CLK)
		sdmmcIOMux(cfg.CMD)
		sdmmcIOMux(cfg.D0)
		if cfg.Width == 4 {
			sdmmcIOMux(cfg.D1)
			sdmmcIOMux(cfg.D2)
		}
	} else {
		cfg.CLK.Configure(PinConfig{Mode: PinOutput})
		cfg.CLK.mux().SetBits(sdmmcIOMuxDrive << esp.IO_MUX_GPIO_FUN_DRV_Pos)
		cfg.CLK.outFunc().Set(sdmmcSig1CLK)
		sdmmcBoth(cfg.CMD, sdmmcSig1CMD)
		sdmmcBoth(cfg.D0, sdmmcSig1D0)
		if cfg.Width == 4 {
			sdmmcBoth(cfg.D1, sdmmcSig1D0+1)
			sdmmcBoth(cfg.D2, sdmmcSig1D0+2)
		}
	}
	if cfg.Width == 4 {
		cfg.D3.Configure(PinConfig{Mode: PinOutput})
		cfg.D3.High()
	}
	sdmmcIn(sdmmcMatrixOne, sdmmcSigCardInt[slot])
	sdmmcIn(sdmmcMatrixZero, sdmmcSigDetect[slot])
	sdmmcIn(sdmmcMatrixZero, sdmmcSigProtect[slot])
}

// sdmmcRouteD3 gives D3 to the host, once the bus is 4 bits.
func sdmmcRouteD3(cfg SDMMCConfig) {
	if cfg.Slot == 0 {
		sdmmcIOMux(cfg.D3)
	} else {
		sdmmcBoth(cfg.D3, sdmmcSig1D0+3)
	}
}

// sdmmcIOMux gives a pin to the host's slot 0, as configure_pin_iomux does, with a pull-up.
func sdmmcIOMux(p Pin) {
	p.mux().Set(sdmmcIOMuxFunc<<esp.IO_MUX_GPIO_MCU_SEL_Pos | sdmmcIOMuxDrive<<esp.IO_MUX_GPIO_FUN_DRV_Pos |
		esp.IO_MUX_GPIO_FUN_IE | esp.IO_MUX_GPIO_FUN_WPU)
}

// sdmmcBoth routes signal both ways through the matrix: out to p, its output enable the host's, and in from p, which is pulled up.
func sdmmcBoth(p Pin, signal uint32) {
	p.Configure(PinConfig{Mode: PinInputPullup})
	p.mux().SetBits(sdmmcIOMuxDrive << esp.IO_MUX_GPIO_FUN_DRV_Pos)
	p.outFunc().Set(signal)
	sdmmcIn(uint32(p), signal)
}

// sdmmcIn feeds an input signal from a pin, or one of the matrix's constants.
func sdmmcIn(pin, signal uint32) {
	inFunc(signal).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | pin)
}

// sdmmcInternal says whether p is in the ESP32-P4's L2MEM, past which it's cache. See esp32p4-psram.ld.
func sdmmcInternal(p unsafe.Pointer) bool {
	a := uintptr(p)
	return a >= 0x4FF00000 && a < 0x4FF80000
}

// sdmmcForCPU is DMA memory as the CPU reaches it: uncached, so it reads what the DMA wrote, and the DMA what it wrote.
func sdmmcForCPU(b []byte) []byte {
	return unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(&b[0]), sdmmcNonCacheable)), len(b))
}

// sdmmcDMAAddress is where the DMA finds what the CPU has at p, cached or not.
func sdmmcDMAAddress(p unsafe.Pointer) uint32 {
	a := uintptr(p)
	if a >= 0x4FF00000+sdmmcNonCacheable {
		a -= sdmmcNonCacheable
	}
	return uint32(a)
}

// sdmmcDescsForCPU is the descriptor chain as the CPU writes it: uncached, so the DMA sees it.
func sdmmcDescsForCPU() *[sdmmcMaxDescs]sdmmcDesc {
	return (*[sdmmcMaxDescs]sdmmcDesc)(unsafe.Add(unsafe.Pointer(&sdmmcDescs), sdmmcNonCacheable))
}
