//go:build esp32p4

package machine

import (
	"device/esp"
	"runtime/volatile"
)

// See ESP-IDF soc/esp32p4/include/soc/gpio_sig_map.h.
const (
	signalI2C0SCL = 68
	signalI2C0SDA = 69
)

var I2C0 = &I2C{
	Bus:     esp.I2C0,
	funcSCL: signalI2C0SCL,
	funcSDA: signalI2C0SDA,
}

// enableI2C0PeriphClock selects the crystal with no divider, which supports all
// standard bus speeds. See ESP-IDF hal/esp32p4/include/hal/i2c_ll.h.
func enableI2C0PeriphClock() {
	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL2_REG_I2C0_APB_CLK_EN(1)
	esp.HP_SYS_CLKRST.SetHP_RST_EN1_REG_RST_EN_I2C0(1)
	esp.HP_SYS_CLKRST.SetHP_RST_EN1_REG_RST_EN_I2C0(0)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL10_REG_I2C0_CLK_SRC_SEL(i2cClkSource)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL10_REG_I2C0_CLK_DIV_NUM(0)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL10_REG_I2C0_CLK_DIV_NUMERATOR(0)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL10_REG_I2C0_CLK_DIV_DENOMINATOR(0)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL10_REG_I2C0_CLK_EN(1)
}

func initI2CExt1Clock() {}

func (p Pin) pinReg() *volatile.Register32 {
	return p.pin()
}

func (p Pin) pullUp() {
	p.mux().SetBits(esp.IO_MUX_GPIO_FUN_WPU)
}

// configure sets the pin mode and connects it to a GPIO matrix signal.
func (p Pin) configure(config PinConfig, signal uint32) {
	p.Configure(config)
	if config.Mode == PinOutput {
		p.outFunc().Set(signal)
	} else {
		inFunc(signal).Set(esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(p))
	}
}
