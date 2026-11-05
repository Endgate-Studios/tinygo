//go:build esp32s3

package machine

// What the ESP32-S3's SD/MMC host needs of its own: see machine_esp32xx_sdmmc.go. Both slots' pins go through the GPIO matrix, and
// its DMA reaches internal data RAM by the CPU's own addresses.

import (
	"device/esp"
	"unsafe"
)

// The host's GPIO matrix signals, slot 0 then slot 1 (soc/esp32s3/include/soc/gpio_sig_map.h), and the matrix's constant inputs
// (soc/gpio_pins.h).
var (
	sdmmcSigCLK     = [2]uint32{172, 173}
	sdmmcSigCMD     = [2]uint32{178, 179}
	sdmmcSigD0      = [2]uint32{180, 213} // D1 to D3 follow
	sdmmcSigDetect  = [2]uint32{194, 195}
	sdmmcSigProtect = [2]uint32{196, 197}
	sdmmcSigCardInt = [2]uint32{198, 199}
)

const (
	sdmmcMatrixOne  = 0x38
	sdmmcMatrixZero = 0x3C
	sdmmcOutOENGPIO = 1 << 10 // FUNCn_OUT_SEL_CFG: output enable from GPIO_ENABLE, not the peripheral
)

// sdmmcHostOn is sdmmc_host_init: the host's clock and reset, and the host reset itself.
func sdmmcHostOn() error {
	esp.SYSTEM.SetPERIP_CLK_EN1_SDIO_HOST_CLK_EN(1)
	esp.SYSTEM.SetPERIP_RST_EN1_SDIO_HOST_RST(1)
	esp.SYSTEM.SetPERIP_RST_EN1_SDIO_HOST_RST(0)
	sdmmcSetHostDiv(2)
	return sdmmcReset()
}

// sdmmcHostOff stops the host's clocks, letting the pins go to another driver.
func sdmmcHostOff() {
	esp.SDHOST.CLK_EDGE_SEL.Set(0)
	esp.SYSTEM.SetPERIP_CLK_EN1_SDIO_HOST_CLK_EN(0)
}

// sdmmcSetHostDiv is sdmmc_ll_set_clock_div, the PLL's 160 MHz as the source, and sdmmc_ll_init_phase_delay.
func sdmmcSetHostDiv(div uint32) {
	hw := esp.SDHOST
	hw.SetCLK_EDGE_SEL_CCLLKIN_EDGE_H(div/2 - 1)
	hw.SetCLK_EDGE_SEL_CCLLKIN_EDGE_L(div - 1)
	hw.SetCLK_EDGE_SEL_CCLLKIN_EDGE_N(div - 1)
	hw.SetCLK_EDGE_SEL_CCLK_EN(1) // clk_sel: PLL 160 MHz
	hw.SetCLK_EDGE_SEL_CCLKIN_EDGE_SLF_SEL(0)
	// 90° on the output clock, for SD's hold time.
	hw.SetCLK_EDGE_SEL_CCLKIN_EDGE_DRV_SEL(1)
	hw.SetCLK_EDGE_SEL_CCLKIN_EDGE_SAM_SEL(0)
	for end := nanotime() + 10_000; nanotime() < end; {
	}
}

// sdmmcRoutePins gives the slot its pins: CMD and the data lines both ways, with pull-ups. A 4-bit slot's D3 is held high as a
// GPIO, which keeps a card in SD mode at CMD0, until sdmmcRouteD3. No card-detect or write-protect pins: the card is there, and
// writable. The SDIO interrupt input is held high.
func sdmmcRoutePins(cfg SDMMCConfig) {
	slot := cfg.Slot
	cfg.CLK.Configure(PinConfig{Mode: PinOutput})
	cfg.CLK.outFunc().Set(sdmmcSigCLK[slot] | sdmmcOutOENGPIO)
	sdmmcBoth(cfg.CMD, sdmmcSigCMD[slot])
	sdmmcBoth(cfg.D0, sdmmcSigD0[slot])
	if cfg.Width == 4 {
		sdmmcBoth(cfg.D1, sdmmcSigD0[slot]+1)
		sdmmcBoth(cfg.D2, sdmmcSigD0[slot]+2)
		cfg.D3.Configure(PinConfig{Mode: PinOutput})
		cfg.D3.High()
	}
	sdmmcIn(sdmmcMatrixOne, sdmmcSigCardInt[slot])
	sdmmcIn(sdmmcMatrixZero, sdmmcSigDetect[slot])
	sdmmcIn(sdmmcMatrixZero, sdmmcSigProtect[slot])
}

// sdmmcRouteD3 gives D3 to the host, once the bus is 4 bits.
func sdmmcRouteD3(cfg SDMMCConfig) {
	sdmmcBoth(cfg.D3, sdmmcSigD0[cfg.Slot]+3)
}

// sdmmcBoth routes signal both ways: out to p, its output enable the host's, and in from p, which is pulled up.
func sdmmcBoth(p Pin, signal uint32) {
	p.Configure(PinConfig{Mode: PinInputPullup})
	p.outFunc().Set(signal)
	sdmmcIn(uint32(p), signal)
}

// sdmmcIn feeds an input signal from a pin, or one of the matrix's constants.
func sdmmcIn(pin, signal uint32) {
	inFunc(signal).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | pin)
}

// sdmmcInternal says whether p is in the ESP32-S3's internal data RAM, which the host's DMA can reach.
func sdmmcInternal(p unsafe.Pointer) bool {
	a := uintptr(p)
	return a >= 0x3FC88000 && a < 0x3FD00000
}

// sdmmcForCPU is DMA memory as the CPU reaches it: the same.
func sdmmcForCPU(b []byte) []byte { return b }

// sdmmcDMAAddress is where the DMA finds what the CPU has at p: the same.
func sdmmcDMAAddress(p unsafe.Pointer) uint32 { return uint32(uintptr(p)) }

// sdmmcDescsForCPU is the descriptor chain as the CPU writes it: the same.
func sdmmcDescsForCPU() *[sdmmcMaxDescs]sdmmcDesc { return &sdmmcDescs }
