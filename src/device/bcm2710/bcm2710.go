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

// IRQ_Type is the ARM interrupt controller (BCM2835 ARM Peripherals section 7.5).
type IRQ_Type struct {
	BASIC_PENDING volatile.Register32
	PENDING1      volatile.Register32
	PENDING2      volatile.Register32
	FIQ_CONTROL   volatile.Register32
	ENABLE1       volatile.Register32
	ENABLE2       volatile.Register32
	ENABLE_BASIC  volatile.Register32
	DISABLE1      volatile.Register32
	DISABLE2      volatile.Register32
	DISABLE_BASIC volatile.Register32
}

// GPU interrupt numbers (BCM2835 ARM Peripherals section 7.5, table "ARM peripherals interrupts").
const (
	IRQ_TIMER1 = 1
	IRQ_TIMER3 = 3
	IRQ_USB    = 9
	IRQ_AUX    = 29
	IRQ_EMMC   = 62
)

// EMMC_Type is the Arasan SD host, wired to the Wi-Fi chip on GPIO34 to 39 (BCM2835 ARM Peripherals section 5).
// Its registers take 32-bit writes only.
type EMMC_Type struct {
	ARG2        volatile.Register32
	BLKSIZECNT  volatile.Register32
	ARG1        volatile.Register32
	CMDTM       volatile.Register32
	RESP        [4]volatile.Register32
	DATA        volatile.Register32
	STATUS      volatile.Register32
	CONTROL0    volatile.Register32
	CONTROL1    volatile.Register32
	INTERRUPT   volatile.Register32
	IRPT_MASK   volatile.Register32
	IRPT_EN     volatile.Register32
	CONTROL2    volatile.Register32
	_           [47]uint32
	SLOTISR_VER volatile.Register32
}

// CM_GP is a general purpose clock's control and divider (BCM2835 ARM Peripherals section 6.3).
type CM_GP struct {
	CTL volatile.Register32
	DIV volatile.Register32
}

const CM_PASSWORD = 0x5a000000

const (
	MBOX_FULL  = 0x80000000
	MBOX_EMPTY = 0x40000000
)

var (
	GPIO = (*GPIO_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0x200000)))
	AUX  = (*AUX_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0x215000)))
	MBOX = (*MBOX_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0xb880)))
	IRQ  = (*IRQ_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0xb200)))
	EMMC = (*EMMC_Type)(unsafe.Pointer(uintptr(PeripheralBase + 0x300000)))
)

// The DMA region start.S maps uncached, for buffers the VideoCore or a DMA engine reads.
const (
	DMAStart = 0x10000000
	DMAEnd   = 0x10800000
)

// FirmwareBundle is where config.txt's initramfs line loads the files tools/rpiboot packs.
const FirmwareBundle = 0x0f000000
