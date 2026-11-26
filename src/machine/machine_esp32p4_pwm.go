//go:build esp32p4

package machine

import (
	"device/esp"
	"runtime/volatile"
	"unsafe"
)

// LEDC PWM on the ESP32-P4, on the 40 MHz crystal: 4 timers, 8 channels. The sequence is ESP-IDF's
// esp_driver_ledc/src/ledc.c and hal/esp32p4/include/hal/ledc_ll.h. The rest is in machine_esp32xx_pwm.go.

const (
	ledcApbClock = 40_000000

	// LEDC_LS_SIG_OUT_PAD_OUT0_IDX in soc/esp32p4/include/soc/gpio_sig_map.h.
	ledcSigOut0 = 126

	ledcChannelStride = 0x14
	ledcTimerStride   = 8
	ledcGammaRAM      = 0x500D3400 // LEDC_GAMMA_RAM in soc/esp32p4/ld/esp32p4.peripherals.ld
	ledcGammaChannel  = 0x40       // 16 entries a channel
)

var (
	PWM0 = &LEDCPWM{SigOutBase: ledcSigOut0, NumChannels: 8, timerNum: 0}
	PWM1 = &LEDCPWM{SigOutBase: ledcSigOut0, NumChannels: 8, timerNum: 1}
	PWM2 = &LEDCPWM{SigOutBase: ledcSigOut0, NumChannels: 8, timerNum: 2}
	PWM3 = &LEDCPWM{SigOutBase: ledcSigOut0, NumChannels: 8, timerNum: 3}
)

func ledcReg(addr uintptr) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(addr))
}

// enableClock turns the LEDC hardware on, on the crystal. The reset clears every timer and channel,
// so it runs only once.
func (pwm *LEDCPWM) enableClock() {
	if !ledcStarted {
		esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL3_REG_LEDC_APB_CLK_EN(1)
		esp.HP_SYS_CLKRST.SetHP_RST_EN1_REG_RST_EN_LEDC(1)
		esp.HP_SYS_CLKRST.SetHP_RST_EN1_REG_RST_EN_LEDC(0)
		esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL22_REG_LEDC_CLK_SRC_SEL(0)
		esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL22_REG_LEDC_CLK_EN(1)
		ledcStarted = true
	}
	esp.LEDC.SetCONF_CLK_EN(1)
}

func (pwm *LEDCPWM) setTimerConf(dutyRes uint8, divReg uint32) {
	conf := ledcReg(uintptr(unsafe.Pointer(&esp.LEDC.TIMER0_CONF)) + uintptr(pwm.timerNum)*ledcTimerStride)
	value := uint32(dutyRes)<<esp.LEDC_TIMER_CONF_TIMER_DUTY_RES_Pos | divReg<<esp.LEDC_TIMER_CONF_CLK_DIV_TIMER_Pos
	conf.Set(value | esp.LEDC_TIMER_CONF_TIMER_RST)
	conf.Set(value)
	conf.Set(value | esp.LEDC_TIMER_CONF_TIMER_PARA_UP)
}

// chanDisable stops a channel from driving its pin.
func chanDisable(ch uint8) {
	conf0 := ledcReg(uintptr(unsafe.Pointer(&esp.LEDC.CH0_CONF0)) + uintptr(ch)*ledcChannelStride)
	conf0.ClearBits(esp.LEDC_CH_CONF0_SIG_OUT_EN_CH)
	conf0.SetBits(esp.LEDC_CH_CONF0_PARA_UP_CH)
}

// chanOp sets up a channel or changes its duty. A fixed duty is one step of a fade that goes nowhere: an
// entry of the gamma RAM of 1 step, 1 cycle and scale 0, as ledc_set_duty_with_hpoint in ESP-IDF has.
func (pwm *LEDCPWM) chanOp(ch uint8, op ledcChanOp, duty uint32) {
	base := uintptr(unsafe.Pointer(&esp.LEDC.CH0_CONF0)) + uintptr(ch)*ledcChannelStride
	conf0, hpoint, dutyReg, conf1 := ledcReg(base), ledcReg(base+4), ledcReg(base+8), ledcReg(base+12)
	if op == ledcChanOpInit {
		hpoint.Set(0)
		duty = 0
		conf0.Set(uint32(pwm.timerNum) << esp.LEDC_CH_CONF0_TIMER_SEL_CH_Pos)
	}
	const entry = 1 | 1<<1 | 0<<11 | 1<<21 // duty_inc, duty_cycle, scale and duty_num
	ledcReg(ledcGammaRAM + uintptr(ch)*ledcGammaChannel).Set(entry)
	ledcReg(uintptr(unsafe.Pointer(&esp.LEDC.CH0_GAMMA_CONF)) + uintptr(ch)*4).Set(1)
	dutyReg.Set(duty)
	conf0.SetBits(esp.LEDC_CH_CONF0_SIG_OUT_EN_CH)
	conf1.SetBits(esp.LEDC_CH_CONF1_DUTY_START_CH)
	conf0.SetBits(esp.LEDC_CH_CONF0_PARA_UP_CH)
}
