//go:build esp32p4

package machine

import (
	"device/esp"
	"errors"
	"runtime/volatile"
	"sync/atomic"
	"unsafe"
)

var errNoEntropy = errors.New("machine: the SAR ADC entropy source did not start")

// The RNG only has true entropy with a noise source on, and this chip has no radio, so that's the SAR ADC.
// See ESP-IDF docs/en/api-reference/system/random.rst.
const (
	rngOff = iota
	rngStarting
	rngReady
	rngFailed
)

// rngReads is how many reads make a word, to give the RNG at least the 56 us between words of esp_random
// in ESP-IDF esp_hw_support/hw_random.c, at 360 MHz. All are mixed in.
const rngReads = 4096

var rngState atomic.Uint32

// GetRNG returns 32 bits from the hardware random number generator, with its noise source on.
func GetRNG() (uint32, error) {
	for rngState.Load() != rngReady {
		if rngState.CompareAndSwap(rngOff, rngStarting) {
			if startRNGNoise() {
				rngState.Store(rngReady)
			} else {
				rngState.Store(rngFailed)
			}
		}
		if rngState.Load() == rngFailed {
			return 0, errNoEntropy
		}
	}
	var word uint32
	for i := 0; i < rngReads; i++ {
		word ^= esp.LP_SYS.RNG_DATA.Get()
	}
	return word, nil
}

// startRNGNoise is bootloader_random_enable in ESP-IDF
// bootloader_support/src/bootloader_random_esp32p4.c. It stays on: nothing else here uses the ADC.
func startRNGNoise() bool {
	esp.LP_PERI.SetCLK_EN_CK_EN_RNG(1)
	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL2_REG_ADC_APB_CLK_EN(1)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL23_REG_ADC_CLK_EN(1)
	esp.LP_ADC.SetMEAS1_MUX_SAR1_DIG_FORCE(1)
	esp.PMU.SetRF_PWC_XPD_PERIF_I2C(1)

	div := esp.HP_SYS_CLKRST.GetPERI_CLK_CTRL24_REG_ADC_SAR1_CLK_DIV_NUM()
	if div == 0 {
		div = 1
	}
	esp.ADC.SetCTRL_START_FORCE(1)

	// I2C_SAR_ADC_INIT_CODE_VAL, 2166, split in two. See soc/esp32p4/include/soc/regi2c_saradc.h.
	const initCode = 2166
	writeSARMask(sarInitCodeMSB, 3, 0, initCode>>8)
	writeSARMask(sarInitCodeLSB, 7, 0, initCode&0xff)

	// One pattern, repeated: channel 10 at attenuation 3, four times over.
	const item = 10<<2 | 3
	esp.ADC.SAR1_PATT_TAB1.Set(item<<18 | item<<12 | item<<6 | item)
	esp.ADC.SetCTRL_SAR1_PATT_LEN(0)
	esp.ADC.SetCTRL_XPD_SAR1_FORCE(3)
	esp.ADC.SetCTRL_XPD_SAR2_FORCE(3)

	writeSARMask(sarVddGroup1, 4, 4, 1)
	writeSARMask(sarVddGroup1, 3, 0, 0)

	esp.ADC.SetCTRL_START_FORCE(0)
	esp.ADC.SetCTRL2_TIMER_EN(1)
	esp.ADC.SetCTRL2_TIMER_TARGET(div * 25)

	for i := 0; i < 1000000; i++ {
		if esp.ADC.GetINT_RAW_SAR1_DONE_INT_RAW() != 0 {
			esp.ADC.SetINT_CLR_APB_SARADC1_DONE_INT_CLR(1)
			return true
		}
	}
	return false
}

// The SAR ADC's block on the analog I2C bus. See ESP-IDF components/esp_rom/patches/esp_rom_regi2c_esp32p4.c
// and soc/esp32p4/include/soc/regi2c_saradc.h.
const (
	anaMaster       = 0x50124000
	anaMasterConf1  = anaMaster + 0x1c
	anaMasterConf2  = anaMaster + 0x20
	anaMasterClk160 = anaMaster + 0x34
	anaSARSel       = 1 << 7
	anaBusy         = 1 << 25
	anaWrite        = 1 << 24
	anaSARBlock     = 0x69

	sarInitCodeLSB = 0
	sarInitCodeMSB = 1
	sarVddGroup1   = 9
)

func anaReg(addr uintptr) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(addr))
}

// writeSARMask sets bits lsb to msb of a register, which is regi2c_write_mask.
func writeSARMask(addr, msb, lsb, value uint32) {
	esp.LP_PERI.SetCLK_EN_CK_EN_LP_I2CMST(1)
	anaReg(anaMasterClk160).SetBits(1)
	anaReg(anaMasterConf2).ClearBits(0xffffff)
	anaReg(anaMasterConf1).ClearBits(0xffffff)
	anaReg(anaMasterConf2).SetBits(anaSARSel)

	ctrl := anaReg(anaMaster)
	for ctrl.Get()&anaBusy != 0 {
	}
	ctrl.Set(anaSARBlock | addr<<8)
	for ctrl.Get()&anaBusy != 0 {
	}
	data := ctrl.Get() >> 16 & 0xff

	mask := uint32(1<<(msb-lsb+1)-1) << lsb
	data = data&^mask | value<<lsb&mask
	ctrl.Set(anaSARBlock | addr<<8 | anaWrite | data<<16)
	for ctrl.Get()&anaBusy != 0 {
	}
}
