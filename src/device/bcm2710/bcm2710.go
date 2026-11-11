// Package bcm2710 has the registers of the BCM2710 used on the Raspberry Pi Zero 2 W and 3.
// Addresses and layouts are from the BCM2835 ARM Peripherals datasheet, moved to 0x3F000000.
package bcm2710

import (
	"runtime/volatile"
	"unsafe"
)

const (
	PeripheralBase = 0x3f000000
	LocalBase      = 0x40000000

	// BusUncached turns an ARM physical address into one the VideoCore and DMA see,
	// past its L2 cache (BCM2835 ARM Peripherals section 1.2.3).
	BusUncached = 0xc0000000
)

// GPIO_Type is the GPIO block (BCM2835 ARM Peripherals section 6.1).
type GPIO_Type struct {
	GPFSEL   [6]volatile.Register32
	_        uint32
	GPSET    [2]volatile.Register32
	_        uint32
	GPCLR    [2]volatile.Register32
	_        uint32
	GPLEV    [2]volatile.Register32
	_        uint32
	GPEDS    [2]volatile.Register32
	_        uint32
	GPREN    [2]volatile.Register32
	_        uint32
	GPFEN    [2]volatile.Register32
	_        uint32
	GPHEN    [2]volatile.Register32
	_        uint32
	GPLEN    [2]volatile.Register32
	_        uint32
	GPAREN   [2]volatile.Register32
	_        uint32
	GPAFEN   [2]volatile.Register32
	_        uint32
	GPPUD    volatile.Register32
	GPPUDCLK [2]volatile.Register32
}

// AUX_Type is the auxiliary block with the mini UART (BCM2835 ARM Peripherals section 2.1).
type AUX_Type struct {
	IRQ        volatile.Register32
	ENABLES    volatile.Register32
	_          [14]uint32
	MU_IO      volatile.Register32
	MU_IER     volatile.Register32
	MU_IIR     volatile.Register32
	MU_LCR     volatile.Register32
	MU_MCR     volatile.Register32
	MU_LSR     volatile.Register32
	MU_MSR     volatile.Register32
	MU_SCRATCH volatile.Register32
	MU_CNTL    volatile.Register32
	MU_STAT    volatile.Register32
	MU_BAUD    volatile.Register32
}

// MBOX_Type is the ARM to VideoCore mailbox 0 and 1 (from the Raspberry Pi firmware wiki, "Mailboxes").
type MBOX_Type struct {
	READ    volatile.Register32
	_       [3]uint32
	PEEK0   volatile.Register32
	SENDER  volatile.Register32
	STATUS  volatile.Register32
	CONFIG  volatile.Register32
	WRITE   volatile.Register32
	_       [3]uint32
	PEEK1   volatile.Register32
	SENDER1 volatile.Register32
	STATUS1 volatile.Register32
	CONFIG1 volatile.Register32
}

const (
	MBOX_FULL  = 0x80000000
	MBOX_EMPTY = 0x40000000
)

var (
	GPIO = (*GPIO_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0x200000)))
	AUX  = (*AUX_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0x215000)))
	MBOX = (*MBOX_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0xb880)))
)
