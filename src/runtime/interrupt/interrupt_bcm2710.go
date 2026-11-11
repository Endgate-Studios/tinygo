//go:build bcm2710

package interrupt

import (
	"device/arm64"
	"device/bcm2710"
)

// State represents the previous global interrupt state.
type State uintptr

// Disable masks IRQ and FIQ and returns the previous DAIF.
func Disable() (state State) {
	state = State(arm64.AsmFull("mrs {}, daif", nil))
	arm64.Asm("msr daifset, #3")
	return state
}

// Restore puts DAIF back to what Disable returned.
func Restore(state State) {
	arm64.AsmFull("msr daif, {state}", map[string]interface{}{"state": uintptr(state)})
}

var inInterrupt [4]bool

func currentCore() uint32 {
	return uint32(arm64.AsmFull("mrs {}, mpidr_el1", nil)) & 3
}

// In returns whether this core is running an interrupt handler.
func In() bool {
	return inInterrupt[currentCore()]
}

// Enable lets the GPU interrupt irq.num reach the ARM (BCM2835 ARM Peripherals section 7.5).
// GPU interrupts go to core 0, GPU_IRQ_ROUTING's reset value.
func (irq Interrupt) Enable() {
	if irq.num < 32 {
		bcm2710.IRQ.ENABLE1.Set(1 << uint(irq.num))
	} else {
		bcm2710.IRQ.ENABLE2.Set(1 << uint(irq.num-32))
	}
}

// crossCore is the runtime's handler for this core's mailbox 0.
//
//go:linkname crossCore runtime.bcm2710CrossCore
func crossCore(core uint32)

//export handleInterrupt
func handleInterrupt() {
	core := currentCore()
	inInterrupt[core] = true
	source := bcm2710.LOCAL.IRQ_SOURCE[core].Get()
	if source&bcm2710.LOCAL_IRQ_CNTPNS != 0 {
		// The sleep that set the timer checks the time itself, so stopping it is enough.
		arm64.Asm("msr cntp_ctl_el0, xzr")
	}
	if source&bcm2710.LOCAL_IRQ_MAILBOX0 != 0 {
		bits := bcm2710.LOCAL.MAILBOX_CLR[core*4].Get()
		bcm2710.LOCAL.MAILBOX_CLR[core*4].Set(bits)
		crossCore(core)
	}
	if source&bcm2710.LOCAL_IRQ_GPU != 0 {
		pending := uint64(bcm2710.IRQ.PENDING1.Get()) | uint64(bcm2710.IRQ.PENDING2.Get())<<32
		for pending != 0 {
			n := 0
			for pending&(1<<uint(n)) == 0 {
				n++
			}
			pending &^= 1 << uint(n)
			callInterruptHandler(n)
		}
	}
	inInterrupt[core] = false
}

//go:linkname callHandlers runtime/interrupt.callHandlers
func callHandlers(num int)

// The compiler needs a constant number in each callHandlers call.
func callInterruptHandler(n int) {
	switch n {
	case bcm2710.IRQ_TIMER1:
		callHandlers(bcm2710.IRQ_TIMER1)
	case bcm2710.IRQ_TIMER3:
		callHandlers(bcm2710.IRQ_TIMER3)
	case bcm2710.IRQ_USB:
		callHandlers(bcm2710.IRQ_USB)
	case bcm2710.IRQ_AUX:
		callHandlers(bcm2710.IRQ_AUX)
	case bcm2710.IRQ_EMMC:
		callHandlers(bcm2710.IRQ_EMMC)
	default:
		// Mask what no handler takes, or it fires forever.
		if n < 32 {
			bcm2710.IRQ.DISABLE1.Set(1 << uint(n))
		} else {
			bcm2710.IRQ.DISABLE2.Set(1 << uint(n-32))
		}
	}
}
