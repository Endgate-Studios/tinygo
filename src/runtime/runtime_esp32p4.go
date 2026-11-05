//go:build esp32p4

package runtime

import (
	"device/esp"
	"device/riscv"
	"machine"
	"runtime/interrupt"
	"runtime/volatile"
	"unsafe"
)

// TIMG0 counts at 20MHz, from the 40MHz crystal divided by 2.
const (
	timerDivider = 2
	nsPerTick    = 50
)

// CPU interrupt number used for the TIMG0 timer alarm.
const timerAlarmCPUInterrupt = 9

// wdtKey unlocks the watchdog registers.
// See ESP-IDF components/hal/esp32p4/include/hal/lpwdt_ll.h.
const wdtKey = 0x50D83AA1

//export main
func main() {
	disableWatchdogs()
	selectCrystalClock()
	initTimerClock()

	clearbss()
	interruptInit()
	initTimer()
	initTimerInterrupt()
	initPSRAM()
	run()

	exit(0)
}

// disableWatchdogs clears the whole config registers, because a watchdog that
// only has its enable bit cleared stays armed by flash boot mode.
// See ESP-IDF components/hal/esp32p4/include/hal/lpwdt_ll.h.
func disableWatchdogs() {
	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL2_REG_TIMERGRP0_APB_CLK_EN(1)
	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL2_REG_TIMERGRP1_APB_CLK_EN(1)

	esp.TIMG0.WDTWPROTECT.Set(wdtKey)
	esp.TIMG0.WDTCONFIG0.Set(0)
	esp.TIMG0.SetWDTCONFIG0_WDT_CONF_UPDATE_EN(1)
	esp.TIMG1.WDTWPROTECT.Set(wdtKey)
	esp.TIMG1.WDTCONFIG0.Set(0)
	esp.TIMG1.SetWDTCONFIG0_WDT_CONF_UPDATE_EN(1)
	esp.LP_WDT.WPROTECT.Set(wdtKey)
	esp.LP_WDT.CONFIG0.Set(0)
	esp.LP_WDT.SWD_WPROTECT.Set(wdtKey)
	esp.LP_WDT.SetSWD_CONFIG_SWD_DISABLE(1)
}

// selectCrystalClock runs the CPU and the buses at 40MHz, which needs no PLL.
// See ESP-IDF rtc_clk_cpu_freq_to_xtal in esp_hw_support/port/esp32p4/rtc_clk.c.
func selectCrystalClock() {
	esp.LP_AON_CLKRST.SetLP_AONCLKRST_HP_CLK_CTRL_LP_AONCLKRST_HP_ROOT_CLK_SRC_SEL(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL0_REG_CPU_CLK_DIV_NUM(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL0_REG_CPU_CLK_DIV_NUMERATOR(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL0_REG_CPU_CLK_DIV_DENOMINATOR(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL1_REG_MEM_CLK_DIV_NUM(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL1_REG_SYS_CLK_DIV_NUM(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL2_REG_APB_CLK_DIV_NUM(0)
	esp.HP_SYS_CLKRST.SetROOT_CLK_CTRL0_REG_SOC_CLK_DIV_UPDATE(1)
	for esp.HP_SYS_CLKRST.GetROOT_CLK_CTRL0_REG_SOC_CLK_DIV_UPDATE() != 0 {
	}
}

// initTimerClock selects the crystal for timer 0 of TIMG0.
// See ESP-IDF timer_ll_set_clock_source in hal/esp32p4/include/hal/timer_ll.h.
func initTimerClock() {
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL20_REG_TIMERGRP0_T0_SRC_SEL(0)
	esp.HP_SYS_CLKRST.SetPERI_CLK_CTRL20_REG_TIMERGRP0_T0_CLK_EN(1)
}

// The timer runs in a different clock domain, so wait for the captured value.
// See ESP-IDF timer_ll_trigger_soft_capture in hal/esp32p4/include/hal/timer_ll.h.
func ticks() timeUnit {
	mask := lockTicks()
	esp.TIMG0.T0UPDATE.Set(esp.TIMG_T0UPDATE_UPDATE)
	for esp.TIMG0.T0UPDATE.HasBits(esp.TIMG_T0UPDATE_UPDATE) {
	}
	t := timeUnit(uint64(esp.TIMG0.T0LO.Get()) | uint64(esp.TIMG0.T0HI.Get())<<32)
	unlockTicks(mask)
	return t
}

func init() {
	machine.InitSerial()
}

func abort() {
	for {
		riscv.Asm("wfi")
	}
}

// interruptInit puts the CLIC in non-vectored use with no interrupt level masked.
// See ESP-IDF components/riscv/include/esp_private/interrupt_clic.h.
func interruptInit() {
	riscv.DisableInterrupts()

	const mtvecModeCLIC = 3
	riscv.MTVEC.Set(uintptr(unsafe.Pointer(&_vector_table)) | mtvecModeCLIC)

	// An enabled level interrupt with no handler would fire forever, so turn
	// off the 32 external lines in case the ROM left one on.
	for i := uintptr(0); i < 32; i++ {
		ctrl := (*volatile.Register32)(unsafe.Pointer(clicExternalCtrl + i*4))
		ctrl.ClearBits(clicIE)
	}

	// The low bits of the level field read as ones, with 3 level bits. A
	// load makes the new threshold active.
	const thresholdNoMask = 0x1f << 24
	clicThreshold.Set(thresholdNoMask)
	clicThreshold.Get()

	riscv.MSTATUS.SetBits(riscv.MSTATUS_MIE)
}

var interruptPending volatile.Register8

func signalInterrupt() {
	interruptPending.Set(1)
}

var timerAlarmInterrupt interrupt.Interrupt

func timerAlarmHandler(interrupt.Interrupt) {
	esp.TIMG0.INT_CLR_TIMERS.Set(1)
}

func initTimerInterrupt() {
	esp.INTERRUPT_CORE0.TIMERGRP0_T0_INT_MAP.Set(timerAlarmCPUInterrupt + clicExternalStart)
	esp.TIMG0.INT_ENA_TIMERS.SetBits(1)
	timerAlarmInterrupt = interrupt.New(timerAlarmCPUInterrupt, timerAlarmHandler)
	timerAlarmInterrupt.Enable()
}

// sleepTicks waits until the given number of ticks have elapsed, or until an
// interrupt comes, because it can wake a goroutine.
func sleepTicks(d timeUnit) {
	target := ticks() + d
	esp.TIMG0.T0ALARMLO.Set(uint32(target))
	esp.TIMG0.T0ALARMHI.Set(uint32(target >> 32))
	esp.TIMG0.T0CONFIG.SetBits(esp.TIMG_T0CONFIG_ALARM_EN)

	for interruptPending.Get() == 0 && ticks() < target {
	}
	interruptPending.Set(0)
}

//go:extern _vector_table
var _vector_table [0]uintptr

// External interrupts start at line 16 of the CLIC, and the interrupt matrix
// takes the line number. See ESP-IDF components/riscv/interrupt_clic.c.
const (
	clicExternalStart = 16
	clicExternalCtrl  = 0x20801000 + clicExternalStart*4
	clicIE            = 1 << 8
)

// clicThreshold is CLIC_INT_THRESH_REG, a register on this chip and not a CSR.
// See ESP-IDF components/soc/esp32p4/include/soc/clic_reg.h.
var clicThreshold = (*volatile.Register32)(unsafe.Pointer(uintptr(0x20800008)))
