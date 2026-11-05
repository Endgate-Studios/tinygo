//go:build esp32p4

package machine

import (
	"device/esp"
	"device/riscv"
	"errors"
	"runtime/interrupt"
	"runtime/volatile"
	"sync"
	"unsafe"
)

const deviceName = esp.Device
const maxPin = 55

// CPU interrupt 6 is reserved. See ESP-IDF esp_hw_support/port/esp32p4/esp_cpu_intr.c.
const (
	cpuInterruptFromPin  = 7
	cpuInterruptFromUART = 13

	// The interrupt matrix takes the CLIC line, which is the CPU interrupt plus 16.
	// See ESP-IDF components/riscv/interrupt_clic.c, intr_matrix_route.
	clicExternalStart = 16
)

// CPUFrequency returns the current CPU frequency of the chip. The runtime
// selects the 40MHz crystal at startup.
func CPUFrequency() uint32 {
	return 40e6
}

const (
	PinOutput PinMode = iota
	PinInput
	PinInputPullup
	PinInputPulldown
	PinAnalog
)

const (
	GPIO0  Pin = 0
	GPIO1  Pin = 1
	GPIO2  Pin = 2
	GPIO3  Pin = 3
	GPIO4  Pin = 4
	GPIO5  Pin = 5
	GPIO6  Pin = 6
	GPIO7  Pin = 7
	GPIO8  Pin = 8
	GPIO9  Pin = 9
	GPIO10 Pin = 10
	GPIO11 Pin = 11
	GPIO12 Pin = 12
	GPIO13 Pin = 13
	GPIO14 Pin = 14
	GPIO15 Pin = 15
	GPIO16 Pin = 16
	GPIO17 Pin = 17
	GPIO18 Pin = 18
	GPIO19 Pin = 19
	GPIO20 Pin = 20
	GPIO21 Pin = 21
	GPIO22 Pin = 22
	GPIO23 Pin = 23
	GPIO24 Pin = 24
	GPIO25 Pin = 25
	GPIO26 Pin = 26
	GPIO27 Pin = 27
	GPIO28 Pin = 28
	GPIO29 Pin = 29
	GPIO30 Pin = 30
	GPIO31 Pin = 31
	GPIO32 Pin = 32
	GPIO33 Pin = 33
	GPIO34 Pin = 34
	GPIO35 Pin = 35
	GPIO36 Pin = 36
	GPIO37 Pin = 37
	GPIO38 Pin = 38
	GPIO39 Pin = 39
	GPIO40 Pin = 40
	GPIO41 Pin = 41
	GPIO42 Pin = 42
	GPIO43 Pin = 43
	GPIO44 Pin = 44
	GPIO45 Pin = 45
	GPIO46 Pin = 46
	GPIO47 Pin = 47
	GPIO48 Pin = 48
	GPIO49 Pin = 49
	GPIO50 Pin = 50
	GPIO51 Pin = 51
	GPIO52 Pin = 52
	GPIO53 Pin = 53
	GPIO54 Pin = 54
)

type PinChange uint8

// Pin change interrupt constants for SetInterrupt.
const (
	PinRising PinChange = iota + 1
	PinFalling
	PinToggle
)

// See ESP-IDF soc/esp32p4/include/soc/gpio_sig_map.h and io_mux_reg.h.
const (
	gpioFunctionGPIO = 1
	signalGPIOOut    = 256
	signalUART0      = 10
	signalUART1      = 13
)

// Configure this pin with the given configuration.
func (p Pin) Configure(config PinConfig) {
	if p == NoPin {
		// This simplifies pin configuration in peripherals such as SPI.
		return
	}

	muxConfig := uint32(gpioFunctionGPIO<<esp.IO_MUX_GPIO_MCU_SEL_Pos | 2<<esp.IO_MUX_GPIO_FUN_DRV_Pos)
	if config.Mode != PinAnalog {
		muxConfig |= esp.IO_MUX_GPIO_FUN_IE
	}
	switch config.Mode {
	case PinInputPullup:
		muxConfig |= esp.IO_MUX_GPIO_FUN_WPU
	case PinInputPulldown:
		muxConfig |= esp.IO_MUX_GPIO_FUN_WPD
	}
	p.mux().Set(muxConfig)

	p.outFunc().Set(signalGPIOOut)
	p.setOutputEnable(config.Mode == PinOutput)
}

func (p Pin) setOutputEnable(enable bool) {
	switch {
	case p < 32 && enable:
		esp.GPIO.ENABLE_W1TS.Set(1 << p)
	case p < 32:
		esp.GPIO.ENABLE_W1TC.Set(1 << p)
	case enable:
		esp.GPIO.ENABLE1_W1TS.Set(1 << (p - 32))
	default:
		esp.GPIO.ENABLE1_W1TC.Set(1 << (p - 32))
	}
}

// outFunc returns the FUNCx_OUT_SEL_CFG register of this pin.
func (p Pin) outFunc() *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC0_OUT_SEL_CFG), uintptr(p)*4))
}

// inFunc returns the FUNCy_IN_SEL_CFG register of the given signal. The first
// register in the device file is for signal 1.
func inFunc(signal uint32) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.FUNC1_IN_SEL_CFG), (uintptr(signal)-1)*4))
}

// mux returns the I/O mux configuration register of this pin.
func (p Pin) mux() *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.IO_MUX.GPIO0), uintptr(p)*4))
}

// pin returns the PIN register of this pin.
func (p Pin) pin() *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Add(unsafe.Pointer(&esp.GPIO.PIN0), uintptr(p)*4))
}

// Set the pin to high or low.
// Warning: only use this on an output pin!
func (p Pin) Set(value bool) {
	if value {
		reg, mask := p.portMaskSet()
		reg.Set(mask)
	} else {
		reg, mask := p.portMaskClear()
		reg.Set(mask)
	}
}

// Get returns the current value of a GPIO pin when configured as an input or as
// an output.
func (p Pin) Get() bool {
	if p < 32 {
		return esp.GPIO.IN.Get()&(1<<p) != 0
	}
	return esp.GPIO.IN1.Get()&(1<<(p-32)) != 0
}

// PortMaskSet returns the register and mask to set a given GPIO pin. This can
// be used to implement bit-banged drivers.
//
// Warning: only use this on an output pin!
func (p Pin) PortMaskSet() (*uint32, uint32) {
	reg, mask := p.portMaskSet()
	return &reg.Reg, mask
}

// PortMaskClear returns the register and mask to clear a given GPIO pin. This
// can be used to implement bit-banged drivers.
//
// Warning: only use this on an output pin!
func (p Pin) PortMaskClear() (*uint32, uint32) {
	reg, mask := p.portMaskClear()
	return &reg.Reg, mask
}

func (p Pin) portMaskSet() (*volatile.Register32, uint32) {
	if p < 32 {
		return &esp.GPIO.OUT_W1TS, 1 << p
	}
	return &esp.GPIO.OUT1_W1TS, 1 << (p - 32)
}

func (p Pin) portMaskClear() (*volatile.Register32, uint32) {
	if p < 32 {
		return &esp.GPIO.OUT_W1TC, 1 << p
	}
	return &esp.GPIO.OUT1_W1TC, 1 << (p - 32)
}

// SetInterrupt sets an interrupt to be executed when a particular pin changes
// state. The pin should already be configured as an input, including a pull up
// or down if no external pull is provided.
//
// You can pass a nil func to unset the pin change interrupt. If you do so,
// the change parameter is ignored and can be set to any value (such as 0).
// If the pin is already configured with a callback, you must first unset
// this pins interrupt before you can set a new callback.
func (p Pin) SetInterrupt(change PinChange, callback func(Pin)) (err error) {
	if p >= maxPin {
		return ErrInvalidInputPin
	}

	if callback == nil {
		p.pin().ClearBits(esp.GPIO_PIN_INT_TYPE_Msk | esp.GPIO_PIN_INT_ENA_Msk)
		pinCallbacks[p] = nil
		return nil
	}

	if pinCallbacks[p] != nil {
		return ErrNoPinChangeChannel
	}
	pinCallbacks[p] = callback

	onceSetupPinInterrupt.Do(func() {
		err = setupPinInterrupt()
	})
	if err != nil {
		return err
	}

	// Bit 0 of INT_ENA routes the interrupt to core 0.
	// See ESP-IDF hal/esp32p4/include/hal/gpio_ll.h.
	p.pin().Set(
		(p.pin().Get() & ^uint32(esp.GPIO_PIN_INT_TYPE_Msk|esp.GPIO_PIN_INT_ENA_Msk)) |
			uint32(change)<<esp.GPIO_PIN_INT_TYPE_Pos | uint32(1)<<esp.GPIO_PIN_INT_ENA_Pos)

	return nil
}

var (
	pinCallbacks          [maxPin]func(Pin)
	onceSetupPinInterrupt sync.Once
)

func setupPinInterrupt() error {
	esp.INTERRUPT_CORE0.GPIO_INT0_MAP.Set(cpuInterruptFromPin + clicExternalStart)
	return interrupt.New(cpuInterruptFromPin, func(interrupt.Interrupt) {
		status := esp.GPIO.STATUS.Get()
		esp.GPIO.STATUS_W1TC.Set(status)
		status1 := esp.GPIO.STATUS1.Get()
		esp.GPIO.STATUS1_W1TC.Set(status1)
		for i := 0; i < maxPin; i++ {
			bits, bit := status, uint(i)
			if i >= 32 {
				bits, bit = status1, uint(i-32)
			}
			if bits&(1<<bit) != 0 && pinCallbacks[i] != nil {
				pinCallbacks[i](Pin(i))
			}
		}
	}).Enable()
}

// enableVO4 turns on the VO4 on-chip regulator at 3.3V. It is off at reset.
// See ESP-IDF esp_ldo_acquire_channel and hal/esp32p4/include/hal/ldo_ll.h.
func enableVO4() {
	esp.PMU.SetEXT_LDO_P1_0P2A_ANA_ANA_0P2A_DREF_1(15)
	esp.PMU.SetEXT_LDO_P1_0P2A_ANA_ANA_0P2A_MUL_1(7)
	esp.PMU.SetEXT_LDO_P1_0P2A__0P2A_TIEH_SEL_1(0)
	esp.PMU.SetEXT_LDO_P1_0P2A__0P2A_TIEH_1(0)
	esp.PMU.SetEXT_LDO_P1_0P2A__0P2A_FORCE_TIEH_SEL_1(1)
	esp.PMU.SetEXT_LDO_P1_0P2A_ANA_ANA_0P2A_EN_VDET_1(1)
	esp.PMU.SetEXT_LDO_P1_0P2A__0P2A_XPD_1(1)
}

var (
	DefaultUART = UART0

	UART0  = &_UART0
	_UART0 = UART{Bus: esp.UART0, Buffer: NewRingBuffer()}
	UART1  = &_UART1
	_UART1 = UART{Bus: esp.UART1, Buffer: NewRingBuffer()}

	onceUart            = sync.Once{}
	errSamePins         = errors.New("UART: invalid pin combination")
	errWrongUART        = errors.New("UART: unsupported UARTn")
	errWrongBitSize     = errors.New("UART: invalid data size")
	errWrongStopBitSize = errors.New("UART: invalid bit size")
)

type UART struct {
	Bus                  *esp.UART_Type
	Buffer               *RingBuffer
	ParityErrorDetected  bool // set when parity error detected
	DataErrorDetected    bool // set when data corruption detected
	DataOverflowDetected bool // set when data overflow detected in UART FIFO buffer or RingBuffer
}

const (
	defaultDataBits = 8
	defaultStopBit  = 1
	defaultParity   = ParityNone

	uartInterrupts = esp.UART_INT_ENA_RXFIFO_FULL_INT_ENA |
		esp.UART_INT_ENA_PARITY_ERR_INT_ENA |
		esp.UART_INT_ENA_FRM_ERR_INT_ENA |
		esp.UART_INT_ENA_RXFIFO_OVF_INT_ENA |
		esp.UART_INT_ENA_GLITCH_DET_INT_ENA

	uartClockFreq = 40e6
)

func (uart *UART) Configure(config UARTConfig) error {
	if config.BaudRate == 0 {
		config.BaudRate = 115200
	}

	// Default pins. See ESP-IDF soc/esp32p4/include/soc/uart_pins.h.
	var signal uint32
	var intrMap *volatile.Register32
	switch uart.Bus {
	case esp.UART0:
		if config.TX == 0 && config.RX == 0 {
			config.TX, config.RX = GPIO37, GPIO38
		}
		signal, intrMap = signalUART0, &esp.INTERRUPT_CORE0.UART0_INT_MAP
	case esp.UART1:
		if config.TX == 0 && config.RX == 0 {
			config.TX, config.RX = GPIO10, GPIO11
		}
		signal, intrMap = signalUART1, &esp.INTERRUPT_CORE0.UART1_INT_MAP
	default:
		return errWrongUART
	}
	if config.TX == config.RX {
		return errSamePins
	}

	uart.initClock()
	uart.Bus.SetCLK_CONF_TX_SCLK_EN(0)
	uart.Bus.SetCLK_CONF_RX_SCLK_EN(0)

	uart.SetBaudRate(config.BaudRate)
	uart.SetFormat(defaultDataBits, defaultStopBit, defaultParity)

	uart.Bus.SetRS485_CONF_SYNC_RS485_EN(0)
	uart.Bus.SetRS485_CONF_SYNC_RS485TX_RX_EN(0)
	uart.Bus.SetRS485_CONF_SYNC_RS485RXBY_TX_EN(0)
	uart.Bus.SetCONF0_SYNC_IRDA_EN(0)
	uart.Bus.SetCONF0_SYNC_TX_FLOW_EN(0)
	uart.update()

	uart.setupPins(config, signal)
	uart.configureInterrupt(intrMap)
	uart.enableTransmitter()
	uart.enableReceiver()
	uart.update()

	uart.Bus.SetCLK_CONF_TX_SCLK_EN(1)
	uart.Bus.SetCLK_CONF_RX_SCLK_EN(1)
	return nil
}

// update moves the values of the _SYNC registers to the core clock domain.
// See ESP-IDF uart_ll_update in hal/esp32p4/include/hal/uart_ll.h.
func (uart *UART) update() {
	for uart.Bus.GetREG_UPDATE() != 0 {
		riscv.Asm("nop")
	}
	uart.Bus.SetREG_UPDATE(1)
}

func (uart *UART) SetFormat(dataBits, stopBits int, parity UARTParity) error {
	if dataBits < 5 {
		return errWrongBitSize
	}
	if stopBits > 1 {
		return errWrongStopBitSize
	}
	uart.Bus.SetCONF0_SYNC_BIT_NUM(uint32(dataBits - 5))
	uart.Bus.SetCONF0_SYNC_STOP_BIT_NUM(uint32(stopBits))
	switch parity {
	case ParityNone:
		uart.Bus.SetCONF0_SYNC_PARITY_EN(0)
	case ParityEven:
		uart.Bus.SetCONF0_SYNC_PARITY_EN(1)
		uart.Bus.SetCONF0_SYNC_PARITY(0)
	case ParityOdd:
		uart.Bus.SetCONF0_SYNC_PARITY_EN(1)
		uart.Bus.SetCONF0_SYNC_PARITY(1)
	}
	return nil
}

// initClock selects the 40MHz crystal and resets the UART. Source 0 is the
// crystal. See ESP-IDF hal/esp32p4/include/hal/uart_ll.h.
func (uart *UART) initClock() {
	clk := esp.HP_SYS_CLKRST
	switch uart.Bus {
	case esp.UART0:
		clk.SetSOC_CLK_CTRL1_REG_UART0_SYS_CLK_EN(1)
		clk.SetSOC_CLK_CTRL2_REG_UART0_APB_CLK_EN(1)
		clk.SetPERI_CLK_CTRL110_REG_UART0_CLK_SRC_SEL(0)
		clk.SetPERI_CLK_CTRL110_REG_UART0_CLK_EN(1)
		clk.SetHP_RST_EN1_REG_RST_EN_UART0_APB(1)
		clk.SetHP_RST_EN1_REG_RST_EN_UART0_APB(0)
		clk.SetPERI_CLK_CTRL111_REG_UART0_SCLK_DIV_NUMERATOR(0)
		clk.SetPERI_CLK_CTRL111_REG_UART0_SCLK_DIV_DENOMINATOR(0)
	case esp.UART1:
		clk.SetSOC_CLK_CTRL1_REG_UART1_SYS_CLK_EN(1)
		clk.SetSOC_CLK_CTRL2_REG_UART1_APB_CLK_EN(1)
		clk.SetPERI_CLK_CTRL111_REG_UART1_CLK_SRC_SEL(0)
		clk.SetPERI_CLK_CTRL111_REG_UART1_CLK_EN(1)
		clk.SetHP_RST_EN1_REG_RST_EN_UART1_APB(1)
		clk.SetHP_RST_EN1_REG_RST_EN_UART1_APB(0)
		clk.SetPERI_CLK_CTRL112_REG_UART1_SCLK_DIV_NUMERATOR(0)
		clk.SetPERI_CLK_CTRL112_REG_UART1_SCLK_DIV_DENOMINATOR(0)
	}
}

// SetBaudRate sets the baud rate. See ESP-IDF uart_ll_set_baudrate.
func (uart *UART) SetBaudRate(baudRate uint32) {
	maxDiv := uint32(1<<12 - 1)
	sclkDiv := (uartClockFreq + maxDiv*baudRate - 1) / (maxDiv * baudRate)
	clkDiv := (uartClockFreq << 4) / (baudRate * sclkDiv)
	uart.Bus.SetCLKDIV_SYNC_CLKDIV(clkDiv >> 4)
	uart.Bus.SetCLKDIV_SYNC_CLKDIV_FRAG(clkDiv & 0xf)
	switch uart.Bus {
	case esp.UART0:
		esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL111_REG_UART0_SCLK_DIV_NUM(sclkDiv - 1)
	case esp.UART1:
		esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL112_REG_UART1_SCLK_DIV_NUM(sclkDiv - 1)
	}
	uart.update()
}

func (uart *UART) setupPins(config UARTConfig, signal uint32) {
	config.RX.Configure(PinConfig{Mode: PinInputPullup})
	config.TX.Configure(PinConfig{Mode: PinOutput})

	out := signal
	if config.InvertTX {
		out |= esp.GPIO_FUNC_OUT_SEL_CFG_INV_SEL
	}
	config.TX.outFunc().Set(out)

	in := esp.GPIO_FUNC_IN_SEL_CFG_SEL | uint32(config.RX)
	if config.InvertRX {
		in |= esp.GPIO_FUNC_IN_SEL_CFG_IN_INV_SEL
	}
	inFunc(signal).Set(in)
}

func (uart *UART) configureInterrupt(intrMapReg *volatile.Register32) {
	uart.Bus.INT_ENA.ClearBits(0x0ffff)

	intrMapReg.Set(cpuInterruptFromUART + clicExternalStart)
	onceUart.Do(func() {
		_ = interrupt.New(cpuInterruptFromUART, func(i interrupt.Interrupt) {
			UART0.serveInterrupt(0)
			UART1.serveInterrupt(1)
		}).Enable()
	})
}

func (uart *UART) serveInterrupt(num int) {
	flags := uart.Bus.INT_ST.Get()
	if flags&uartInterrupts == 0 {
		return
	}

	uart.Bus.INT_ENA.ClearBits(uartInterrupts)

	if flags&esp.UART_INT_ENA_RXFIFO_FULL_INT_ENA != 0 {
		for uart.Bus.GetSTATUS_RXFIFO_CNT() > 0 {
			if !uart.Buffer.Put(byte(uart.Bus.GetFIFO_RXFIFO_RD_BYTE())) {
				uart.DataOverflowDetected = true
			}
		}
	}
	if flags&esp.UART_INT_ENA_PARITY_ERR_INT_ENA != 0 {
		uart.ParityErrorDetected = true
	}
	if flags&(esp.UART_INT_ENA_FRM_ERR_INT_ENA|esp.UART_INT_ENA_GLITCH_DET_INT_ENA) != 0 {
		uart.DataErrorDetected = true
	}
	if flags&esp.UART_INT_ENA_RXFIFO_OVF_INT_ENA != 0 {
		uart.DataOverflowDetected = true
	}

	uart.Bus.INT_CLR.Set(flags)
	uart.Bus.INT_ENA.Set(uartInterrupts)
}

func (uart *UART) enableTransmitter() {
	uart.Bus.SetCONF0_SYNC_TXFIFO_RST(1)
	uart.Bus.SetCONF0_SYNC_TXFIFO_RST(0)
	uart.Bus.SetCONF1_TXFIFO_EMPTY_THRHD(10)
}

func (uart *UART) enableReceiver() {
	uart.Bus.SetCONF0_SYNC_RXFIFO_RST(1)
	uart.Bus.SetCONF0_SYNC_RXFIFO_RST(0)
	uart.Bus.SetCONF1_RXFIFO_FULL_THRHD(1)
	uart.Bus.INT_ENA.SetBits(uartInterrupts)
}

func (uart *UART) writeByte(b byte) error {
	for (uart.Bus.STATUS.Get()&esp.UART_STATUS_TXFIFO_CNT_Msk)>>esp.UART_STATUS_TXFIFO_CNT_Pos >= 128 {
	}
	uart.Bus.FIFO.Set(uint32(b))
	return nil
}

func (uart *UART) flush() {}
