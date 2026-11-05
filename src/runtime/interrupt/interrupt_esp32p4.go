//go:build esp32p4

package interrupt

import (
	"device/riscv"
	"errors"
	"runtime/volatile"
	"unsafe"
)

//go:extern tinygo_saved_ra
var tinygo_saved_ra uintptr

// See ESP-IDF components/soc/esp32p4/include/soc/clic_reg.h.
const (
	clicCtrlBase      = 0x20801000
	clicExternalStart = 16

	clicIE          = 1 << 8
	clicModeMachine = 3 << 22
	clicLevel1      = (1<<5 | 0x1f) << 24

	// CPU interrupt 6 is reserved in ESP-IDF esp_hw_support/port/esp32p4/esp_cpu_intr.c.
	cpuInterruptReserved = 6

	// See ESP-IDF components/riscv/include/esp_private/vectors_const.h.
	mcauseInterrupt  = 1 << 31
	mcauseReasonMask = 0xfff

	// mintstatus is at a non-standard address on this chip, with the current
	// level in the top byte. See ESP-IDF esp_private/interrupt_clic.h.
	mintstatus    = riscv.CSR(0x346)
	mintstatusMIL = 24
)

func clicCtrl(num int) *volatile.Register32 {
	return (*volatile.Register32)(unsafe.Pointer(uintptr(clicCtrlBase + (num+clicExternalStart)*4)))
}

// In returns whether the system is currently in an interrupt. It reads the
// interrupt level, because mcause keeps its value after the handler returns.
func In() bool {
	return mintstatus.Get()>>mintstatusMIL != 0
}

// Enable enables a level triggered CPU interrupt, in the range 1 through 31.
func (i Interrupt) Enable() error {
	if i.num < 1 || i.num > 31 || i.num == cpuInterruptReserved {
		return errors.New("interrupt must be in range of 1 through 31, and not 6")
	}
	mask := riscv.DisableInterrupts()
	clicCtrl(i.num).Set(clicLevel1 | clicModeMachine | clicIE)
	riscv.Asm("fence")
	riscv.EnableInterrupts(mask)
	return nil
}

//go:linkname callHandlers runtime/interrupt.callHandlers
func callHandlers(num int)

//go:linkname signalInterrupt runtime.signalInterrupt
func signalInterrupt()

const (
	IRQNUM_1 = 1 + iota
	IRQNUM_2
	IRQNUM_3
	IRQNUM_4
	IRQNUM_5
	IRQNUM_6
	IRQNUM_7
	IRQNUM_8
	IRQNUM_9
	IRQNUM_10
	IRQNUM_11
	IRQNUM_12
	IRQNUM_13
	IRQNUM_14
	IRQNUM_15
	IRQNUM_16
	IRQNUM_17
	IRQNUM_18
	IRQNUM_19
	IRQNUM_20
	IRQNUM_21
	IRQNUM_22
	IRQNUM_23
	IRQNUM_24
	IRQNUM_25
	IRQNUM_26
	IRQNUM_27
	IRQNUM_28
	IRQNUM_29
	IRQNUM_30
	IRQNUM_31
)

//go:inline
func callHandler(n int) {
	switch n {
	case IRQNUM_1:
		callHandlers(IRQNUM_1)
	case IRQNUM_2:
		callHandlers(IRQNUM_2)
	case IRQNUM_3:
		callHandlers(IRQNUM_3)
	case IRQNUM_4:
		callHandlers(IRQNUM_4)
	case IRQNUM_5:
		callHandlers(IRQNUM_5)
	case IRQNUM_7:
		callHandlers(IRQNUM_7)
	case IRQNUM_8:
		callHandlers(IRQNUM_8)
	case IRQNUM_9:
		callHandlers(IRQNUM_9)
	case IRQNUM_10:
		callHandlers(IRQNUM_10)
	case IRQNUM_11:
		callHandlers(IRQNUM_11)
	case IRQNUM_12:
		callHandlers(IRQNUM_12)
	case IRQNUM_13:
		callHandlers(IRQNUM_13)
	case IRQNUM_14:
		callHandlers(IRQNUM_14)
	case IRQNUM_15:
		callHandlers(IRQNUM_15)
	case IRQNUM_16:
		callHandlers(IRQNUM_16)
	case IRQNUM_17:
		callHandlers(IRQNUM_17)
	case IRQNUM_18:
		callHandlers(IRQNUM_18)
	case IRQNUM_19:
		callHandlers(IRQNUM_19)
	case IRQNUM_20:
		callHandlers(IRQNUM_20)
	case IRQNUM_21:
		callHandlers(IRQNUM_21)
	case IRQNUM_22:
		callHandlers(IRQNUM_22)
	case IRQNUM_23:
		callHandlers(IRQNUM_23)
	case IRQNUM_24:
		callHandlers(IRQNUM_24)
	case IRQNUM_25:
		callHandlers(IRQNUM_25)
	case IRQNUM_26:
		callHandlers(IRQNUM_26)
	case IRQNUM_27:
		callHandlers(IRQNUM_27)
	case IRQNUM_28:
		callHandlers(IRQNUM_28)
	case IRQNUM_29:
		callHandlers(IRQNUM_29)
	case IRQNUM_30:
		callHandlers(IRQNUM_30)
	case IRQNUM_31:
		callHandlers(IRQNUM_31)
	}
}

// handleInterrupt runs with interrupts off, so handlers do not nest and the
// previous interrupt level in mcause is always 0.
//
//export handleInterrupt
func handleInterrupt() {
	mcause := riscv.MCAUSE.Get()
	reason := int(mcause & mcauseReasonMask)

	if mcause&mcauseInterrupt == 0 || reason < clicExternalStart {
		handleException(mcause)
		return
	}

	mstatus := riscv.MSTATUS.Get()
	mepc := riscv.MEPC.Get()

	callHandler(reason - clicExternalStart)
	signalInterrupt()

	// mret restores the interrupt level from mcause, so it must keep its value.
	// See ESP-IDF components/riscv/vectors.S, _interrupt_handler.
	riscv.MCAUSE.Set(mcause)
	riscv.MSTATUS.Set(mstatus)
	riscv.MEPC.Set(mepc)
}

func handleException(mcause uintptr) {
	println("*** Exception:     pc:", riscv.MEPC.Get())
	println("*** Exception:   code:", uint32(mcause&mcauseReasonMask))
	println("*** Exception: mcause:", mcause)
	println("*** Exception:     ra:", tinygo_saved_ra)
	switch uint32(mcause & mcauseReasonMask) {
	case riscv.InstructionAccessFault:
		println("***    virtual address:", riscv.MTVAL.Get())
	case riscv.IllegalInstruction:
		println("***            opcode:", riscv.MTVAL.Get())
	case riscv.LoadAccessFault:
		println("***      read address:", riscv.MTVAL.Get())
	case riscv.StoreOrAMOAccessFault:
		println("***     write address:", riscv.MTVAL.Get())
	}
	for {
		riscv.Asm("wfi")
	}
}
