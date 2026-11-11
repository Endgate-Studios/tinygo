//go:build bcm2710

package machine

import (
	"device/arm64"
	"device/bcm2710"
	"errors"
	"runtime/volatile"
	"unsafe"
)

const deviceName = "BCM2710"

const (
	PinOutput PinMode = iota
	PinInput
	PinInputPullup
	PinInputPulldown
	PinAlt0
	PinAlt1
	PinAlt2
	PinAlt3
	PinAlt4
	PinAlt5
)

// Function select values per mode (BCM2835 ARM Peripherals section 6.1, GPFSELn).
var fsel = [...]uint32{
	PinOutput:        1,
	PinInput:         0,
	PinInputPullup:   0,
	PinInputPulldown: 0,
	PinAlt0:          4,
	PinAlt1:          5,
	PinAlt2:          6,
	PinAlt3:          7,
	PinAlt4:          3,
	PinAlt5:          2,
}

// Configure sets the pin's function and pull.
func (p Pin) Configure(config PinConfig) {
	if p > 53 || int(config.Mode) >= len(fsel) {
		return
	}
	reg := &bcm2710.GPIO.GPFSEL[p/10]
	shift := uint32(p%10) * 3
	reg.Set(reg.Get()&^(7<<shift) | fsel[config.Mode]<<shift)
	switch config.Mode {
	case PinInputPullup:
		p.setPull(2)
	case PinInputPulldown:
		p.setPull(1)
	default:
		p.setPull(0)
	}
}

// setPull is the GPPUD and GPPUDCLK sequence (BCM2835 ARM Peripherals section 6.1, page 101).
func (p Pin) setPull(pull uint32) {
	bcm2710.GPIO.GPPUD.Set(pull)
	delayCycles(150)
	bcm2710.GPIO.GPPUDCLK[p/32].Set(1 << (p % 32))
	delayCycles(150)
	bcm2710.GPIO.GPPUD.Set(0)
	bcm2710.GPIO.GPPUDCLK[p/32].Set(0)
}

func delayCycles(n int) {
	for i := 0; i < n; i++ {
		arm64.Asm("nop")
	}
}

// Set drives the pin high or low.
func (p Pin) Set(high bool) {
	if high {
		bcm2710.GPIO.GPSET[p/32].Set(1 << (p % 32))
	} else {
		bcm2710.GPIO.GPCLR[p/32].Set(1 << (p % 32))
	}
}

// Get reads the pin's level.
func (p Pin) Get() bool {
	return bcm2710.GPIO.GPLEV[p/32].Get()&(1<<(p%32)) != 0
}

// UART is the mini UART. Its clock is the VPU core clock, which config.txt fixes at 250 MHz.
type UART struct {
	Buffer *RingBuffer
}

const coreClock = 250 * MHz

var (
	DefaultUART = UART0
	UART0       = &_UART0
	_UART0      = UART{Buffer: NewRingBuffer()}
)

// Configure starts the mini UART on its pins at 8N1 (BCM2835 ARM Peripherals section 2.2).
func (uart *UART) Configure(config UARTConfig) error {
	if config.BaudRate == 0 {
		config.BaudRate = 115200
	}
	if config.TX == 0 && config.RX == 0 {
		config.TX, config.RX = UART_TX_PIN, UART_RX_PIN
	}
	aux := bcm2710.AUX
	aux.ENABLES.SetBits(1)
	aux.MU_CNTL.Set(0)
	aux.MU_IER.Set(0)
	aux.MU_LCR.Set(3)
	aux.MU_MCR.Set(0)
	aux.MU_IIR.Set(0xc6)
	aux.MU_BAUD.Set(coreClock/(8*config.BaudRate) - 1)
	config.TX.Configure(PinConfig{Mode: PinAlt5})
	config.RX.Configure(PinConfig{Mode: PinAlt5})
	aux.MU_CNTL.Set(3)
	return nil
}

// WriteByte waits for room in the transmit FIFO and sends c.
func (uart *UART) WriteByte(c byte) error {
	for bcm2710.AUX.MU_LSR.Get()&0x20 == 0 {
	}
	bcm2710.AUX.MU_IO.Set(uint32(c))
	return nil
}

func (uart *UART) Write(data []byte) (int, error) {
	for _, c := range data {
		uart.WriteByte(c)
	}
	return len(data), nil
}

// Buffered polls the receiver, as no interrupt fills the buffer yet.
func (uart *UART) Buffered() int {
	for bcm2710.AUX.MU_LSR.Get()&1 != 0 {
		uart.Buffer.Put(byte(bcm2710.AUX.MU_IO.Get()))
	}
	return int(uart.Buffer.Used())
}

func (uart *UART) ReadByte() (byte, error) {
	uart.Buffered()
	c, ok := uart.Buffer.Get()
	if !ok {
		return 0, errUARTEmpty
	}
	return c, nil
}

func (uart *UART) Read(data []byte) (int, error) {
	n := 0
	for n < len(data) && uart.Buffered() > 0 {
		data[n], _ = uart.Buffer.Get()
		n++
	}
	return n, nil
}

var errUARTEmpty = errors.New("UART buffer empty")

// The power manager's watchdog, as Linux drivers/watchdog/bcm2835_wdt.c drives it.
var (
	pmRSTC = (*volatile.Register32)(unsafe.Pointer(uintptr(bcm2710.PeripheralBase + 0x10001c)))
	pmWDOG = (*volatile.Register32)(unsafe.Pointer(uintptr(bcm2710.PeripheralBase + 0x100024)))
)

const (
	pmPassword        = 0x5a000000
	pmRSTCFullReset   = 0x20
	pmRSTCConfigClear = 0xffffffcf
)

// CPUReset restarts the chip with the watchdog.
func CPUReset() {
	pmWDOG.Set(pmPassword | 10)
	pmRSTC.Set(pmPassword | pmRSTCConfigClear&pmRSTC.Get() | pmRSTCFullReset)
	for {
		arm64.Asm("wfe")
	}
}

// EnterBootloader restarts the chip. With no SD card the boot ROM then waits for rpiboot on USB.
func EnterBootloader() {
	CPUReset()
}
