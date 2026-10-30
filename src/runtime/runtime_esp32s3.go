//go:build esp32s3

package runtime

import (
	"device"
	"device/esp"
	"machine"
	"runtime/volatile"
	"unsafe"
)

// See ESP-IDF components/esp_rom/esp32s3/ld/esp32s3.rom.ld.
//
//export ets_update_cpu_frequency
func romUpdateCPUFrequency(mhz uint32)

//export ets_delay_us
func romDelayUs(us uint32)

//export rom_i2c_writeReg_Mask
func romI2CWriteRegMask(block, hostID, reg, msb, lsb, data uint8)

//export rom_i2c_readReg_Mask
func romI2CReadRegMask(block, hostID, reg, msb, lsb uint8) uint8

// The LDO registers of ESP-IDF v5.3.2 soc/esp32s3/include/soc/regi2c_dig_reg.h.
const (
	i2cDigReg           = 0x6D
	i2cDigRegExtRTCDreg = 4
	i2cDigRegExtDigDreg = 6
)

// The bandgap registers of ESP-IDF v5.3.2 soc/esp32s3/include/soc/regi2c_ulp.h
// and regi2c_lp_bias.h.
const (
	i2cULP            = 0x61
	i2cULPIRResetB    = 0 // bit 0
	i2cULPDoneFlags   = 3 // O_DONE_FLAG bit 0, BG_O_DONE_FLAG bit 3
	i2cULPIRForceCode = 5 // bit 6
	i2cULPExtCode     = 6 // bits 7:0
)

// rtcInit does what ESP-IDF v5.3.2 esp_rtc_init (esp_system/port/soc/esp32s3/clk.c)
// and rtc_init (esp_hw_support/port/esp32s3/rtc_init.c) do for the radio
// at power on, which an image the ROM starts directly doesn't get: the
// bandgap calibration (O-code), which every LDO voltage is set against, the
// SAR I2C's protection from a low RTC LDO, and the SAR power under the
// FSM. Without them the PHY's first calibration and transmit brought the
// chip down. It runs on the XTAL clock, before the PLL, as calibrate_ocode
// wants.
func rtcInit() {
	// "open sar_i2c protect function to avoid sar_i2c reset when rtc_ldo is
	// low." (machine.InitADC sets it too, but only for the ADC.)
	esp.RTC_CNTL.SetANA_CONF_I2C_RESET_POR_FORCE_PD(0)

	// The O-code: from the eFuse with BLK_VERSION_MAJOR 1, as
	// set_ocode_by_efuse, or else calibrated by the hardware, as
	// calibrate_ocode, waiting up to 10 ms for both done flags.
	if efuse(0x6C)&3 == 1 {
		romI2CWriteRegMask(i2cULP, 1, i2cULPExtCode, 7, 0, uint8(efuse(0x6C)>>13))
		romI2CWriteRegMask(i2cULP, 1, i2cULPIRForceCode, 6, 6, 1)
	} else {
		romI2CWriteRegMask(i2cULP, 1, i2cULPIRResetB, 0, 0, 0)
		romI2CWriteRegMask(i2cULP, 1, i2cULPIRResetB, 0, 0, 1)
		for range 1000 {
			if romI2CReadRegMask(i2cULP, 1, i2cULPDoneFlags, 0, 0) != 0 &&
				romI2CReadRegMask(i2cULP, 1, i2cULPDoneFlags, 3, 3) != 0 {
				break
			}
			romDelayUs(10)
		}
	}

	// sar_periph_ctrl_init: SAR power under the FSM.
	esp.SENS.SetSAR_PERI_CLK_GATE_CONF_SARADC_CLK_EN(1)
	esp.SENS.SetSAR_POWER_XPD_SAR_FORCE_XPD_SAR(0)
}

// dbias240 gives the LDO values for 240 MHz, as rtc_set_stored_dbias of ESP-IDF
// v5.3.2 esp_hw_support/port/esp32s3/rtc_init.c, from the eFuse BLOCK1 fields.
func dbias240() (dig, rtc uint32) {
	minor, major := efuse(0x50)>>24&7, efuse(0x6C)&3
	if !(major <= 1 && minor == 1 || major > 1 || major == 1 && minor >= 2) {
		return 28, 28
	}
	w4, w5 := efuse(0x54), efuse(0x58)
	hvt := int32(w5 >> 11 & 0x1f)
	if hvt == 0 {
		return 28, 28
	}
	kRTC := 198 + signed(w4>>13&0x7f, 1<<6)
	kDig := 211 + signed(w4>>20&0x7f, 1<<6)
	vRTC20 := 10181 + signed(w4>>27|(w5&7)<<5, 1<<7)*20
	vDig20 := 10841 + signed(w5>>3&0xff, 1<<7)*20
	d := int32(15)
	for d < 31 && vDig20+kDig*(d-20) < 13000 {
		d++
	}
	d = min(d, hvt+3)
	r := int32(15)
	for r < 31 && vRTC20+kRTC*(r-20) < vDig20+kDig*(d-20)-250 {
		r++
	}
	return uint32(d), uint32(r)
}

func efuse(offset uintptr) uint32 {
	return (*volatile.Register32)(unsafe.Pointer(0x60007000 + offset)).Get()
}

// signed decodes an eFuse value with a sign bit.
func signed(v, sign uint32) int32 {
	if v&sign != 0 {
		return -int32(v & (sign - 1))
	}
	return int32(v)
}

// This is the function called on startup after the flash (IROM/DROM) is
// initialized and the stack pointer has been set.
//
//export main
func main() {
	// This initialization configures the following things:
	// * It disables all watchdog timers. They might be useful at some point in
	//   the future, but will need integration into the scheduler. For now,
	//   they're all disabled.
	// * It sets the CPU frequency to 240MHz, which is the maximum speed allowed
	//   for this CPU. Lower frequencies might be possible in the future, but
	//   running fast and sleeping quickly is often also a good strategy to save
	//   power.
	// TODO: protect certain memory regions, especially the area below the stack
	// to protect against stack overflows. See
	// esp_cpu_configure_region_protection in ESP-IDF.

	// Disable RTC watchdog.
	esp.RTC_CNTL.WDTWPROTECT.Set(0x50D83AA1)
	esp.RTC_CNTL.WDTCONFIG0.Set(0)
	esp.RTC_CNTL.WDTWPROTECT.Set(0x0) // Re-enable write protect

	// Disable Timer 0 watchdog.
	esp.TIMG1.WDTWPROTECT.Set(0x50D83AA1) // write protect
	esp.TIMG1.WDTCONFIG0.Set(0)           // disable TG0 WDT
	esp.TIMG1.WDTWPROTECT.Set(0x0)        // Re-enable write protect

	esp.TIMG0.WDTWPROTECT.Set(0x50D83AA1) // write protect
	esp.TIMG0.WDTCONFIG0.Set(0)           // disable TG0 WDT
	esp.TIMG0.WDTWPROTECT.Set(0x0)        // Re-enable write protect

	// Disable super watchdog.
	esp.RTC_CNTL.SWD_WPROTECT.Set(0x8F1D312A)
	esp.RTC_CNTL.SWD_CONF.Set(esp.RTC_CNTL_SWD_CONF_SWD_DISABLE)
	esp.RTC_CNTL.SWD_WPROTECT.Set(0x0) // Re-enable write protect

	rtcInit()

	// Raise the core voltage for 240 MHz first, as ESP-IDF v5.3.2
	// esp_hw_support/port/esp32s3/rtc_clk.c rtc_clk_cpu_freq_to_pll_mhz does.
	dig, rtc := dbias240()
	romI2CWriteRegMask(i2cDigReg, 1, i2cDigRegExtRTCDreg, 4, 0, uint8(rtc))
	romI2CWriteRegMask(i2cDigReg, 1, i2cDigRegExtDigDreg, 4, 0, uint8(dig))
	romDelayUs(40)
	// All 6 LDO slaves on for 240 MHz (RTC_CNTL_SLAVE_PD, bits 13 to 18).
	(*volatile.Register32)(unsafe.Pointer(uintptr(0x600081FC))).ReplaceBits(0, 0x3F, 13)

	// Change CPU frequency from 20MHz to 80MHz, by switching from the XTAL to the
	// PLL clock source (see table "CPU Clock Frequency" in the reference manual).
	esp.SYSTEM.SetSYSCLK_CONF_SOC_CLK_SEL(1)

	// Change CPU frequency from 80MHz to 240MHz by setting SYSTEM_PLL_FREQ_SEL to
	// 1 and SYSTEM_CPUPERIOD_SEL to 2 (see table "CPU Clock Frequency" in the
	// reference manual).
	// We do this gradually to allow PLL and system to stabilize.
	esp.SYSTEM.SetCPU_PER_CONF_PLL_FREQ_SEL(1)

	// First switch to 160MHz (intermediate step)
	esp.SYSTEM.SetCPU_PER_CONF_CPUPERIOD_SEL(1)
	// Small delay to let PLL stabilize at 160MHz
	for i := 0; i < 1000; i++ {
		_ = esp.SYSTEM.CPU_PER_CONF.Get()
	}

	// Now switch to 240MHz
	esp.SYSTEM.SetCPU_PER_CONF_CPUPERIOD_SEL(2)
	// Small delay to let PLL stabilize at 240MHz
	for i := 0; i < 1000; i++ {
		_ = esp.SYSTEM.CPU_PER_CONF.Get()
	}
	// ROM ets_delay_us counts CPU cycles with the frequency set here, as
	// ESP-IDF esp_rom_set_cpu_ticks_per_us does after a clock change.
	romUpdateCPUFrequency(240)

	// Clear bss. Repeat many times while we wait for cpu/clock to stabilize
	for x := 0; x < 30; x++ {
		clearbss()
	}

	// Initialize PSRAM if numa_psram_* build tag is enabled.
	initPSRAM()

	// Initialize main system timer used for time.Now.
	initTimer()

	// Set up the Xtensa interrupt vector table.
	interruptInit()

	// Initialize timer alarm interrupt for the scheduler.
	initTimerInterrupt()

	// Initialize the heap, call main.main, etc.
	run()

	// Fallback: if main ever returns, hang the CPU.
	exit(0)
}

func init() {
	// Initialize UART.
	machine.InitSerial()

	// Report a PSRAM bring-up failure now that there is somewhere to report it.
	// initPSRAM runs long before this and cannot print anything.
	checkPSRAMInit()
}

func abort() {
	print("abort called\n")
	// lock up forever
	for {
		device.Asm("waiti 0")
	}
}

// interruptInit installs the Xtensa vector table by writing its address
// to the VECBASE special register and ensures all CPU interrupts are
// initially disabled.
func interruptInit() {
	// Disable all CPU interrupts while we configure.
	device.AsmFull("wsr {zero}, INTENABLE", map[string]interface{}{
		"zero": uintptr(0),
	})

	// Write the vector table address to VECBASE (SR 231).
	vecbase := uintptr(unsafe.Pointer(&_vector_table))
	device.AsmFull("wsr {vecbase}, VECBASE", map[string]interface{}{
		"vecbase": vecbase,
	})

	// Clear PS.EXCM and PS.INTLEVEL so that level-1 interrupts can fire.
	// The ROM bootloader leaves PS.EXCM=1 (exception mode), which masks
	// all interrupts at level ≤ EXCMLEVEL (level 1 on ESP32-S3).
	// PS.INTLEVEL may also be non-zero. Both must be 0 for peripheral
	// interrupts to trigger.
	//
	// We also set PS.UM=1 (bit 5) so that level-1 interrupts route to
	// the User exception vector at VECBASE+0x340, where our handler lives.
	// With PS.UM=0 (the ROM default), they would go to the Kernel exception
	// vector at VECBASE+0x300 which is an infinite-loop stub.
	ps := uintptr(device.AsmFull("rsr {}, PS", nil))
	ps &^= 0x1F // clear INTLEVEL (bits 0-3) and EXCM (bit 4)
	ps |= 0x20  // set PS.UM (bit 5) — use User exception vector
	device.AsmFull("wsr {ps}, PS", map[string]interface{}{
		"ps": ps,
	})

	// Synchronize pipeline after writing special registers.
	device.Asm("rsync")
}

//go:extern _vector_table
var _vector_table [0]uintptr

//go:extern _sbss
var _sbss [0]byte

//go:extern _ebss
var _ebss [0]byte
