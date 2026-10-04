//go:build esp32s3 && scheduler.cores

package runtime

import (
	"device"
	"device/esp"
	"internal/task"
	"runtime/interrupt"
	"runtime/volatile"
	"sync/atomic"
	"unsafe"
)

const numCPU = 2

// Level 1 and level triggered on both cores, and not used by machine or espradio.
// See ESP-IDF components/esp_hw_support/port/esp32s3/esp_cpu_intr.c.
const crossCoreCPUInterrupt = 18

var crossCoreInterrupt interrupt.Interrupt

// PRID is 0xCDCD on core 0 and 0xABAB on core 1, see ESP-IDF xt_utils_get_core_id.
func currentCPU() uint32 {
	return uint32(device.AsmFull("rsr {}, PRID", nil)) >> 13 & 1
}

// See ESP-IDF components/esp_rom/esp32s3/ld/esp32s3.rom.ld.
//
//export ets_set_appcpu_boot_addr
func romSetAppCPUBootAddr(addr uint32)

//go:extern call_start_cpu1
var callStartCPU1 [0]byte

//go:extern _stack1_top
var stack1TopSymbol [0]byte

var core1Up atomic.Uint32

// See start_other_core in ESP-IDF components/esp_system/port/cpu_start.c.
func startSecondaryCores() {
	crossCoreInterrupt = interrupt.New(crossCoreCPUInterrupt, crossCoreHandler)
	esp.INTERRUPT_CORE0.SetCPU_INTR_FROM_CPU_0_MAP(crossCoreCPUInterrupt)
	crossCoreInterrupt.Enable()

	esp.RTC_CNTL.SetSW_CPU_STALL_SW_STALL_APPCPU_C1(0)
	esp.RTC_CNTL.SetOPTIONS0_SW_STALL_APPCPU_C0(0)

	// OpenOCD may have started core 1 already. A reset would remove its breakpoints.
	if esp.SYSTEM.GetCORE_1_CONTROL_0_CONTROL_CORE_1_CLKGATE_EN() == 0 {
		esp.SYSTEM.SetCORE_1_CONTROL_0_CONTROL_CORE_1_CLKGATE_EN(1)
		esp.SYSTEM.SetCORE_1_CONTROL_0_CONTROL_CORE_1_RUNSTALL(0)
		esp.SYSTEM.SetCORE_1_CONTROL_0_CONTROL_CORE_1_RESETING(1)
		esp.SYSTEM.SetCORE_1_CONTROL_0_CONTROL_CORE_1_RESETING(0)
	}
	romSetAppCPUBootAddr(uint32(uintptr(unsafe.Pointer(&callStartCPU1))))

	for core1Up.Load() == 0 {
		spinLoopWait()
	}
}

//export tinygo_runCore1
func runCore1() {
	interruptInit()
	romSetAppCPUBootAddr(0)

	esp.INTERRUPT_CORE1.SetCPU_INTR_FROM_CPU_1_MAP(crossCoreCPUInterrupt)
	crossCoreInterrupt.Enable()
	timerAlarmInterrupt.Enable()
	core1Up.Store(1)

	// Do not run goroutines before the GC can stop this core.
	for volatile.LoadUint8((*uint8)(unsafe.Pointer(&secondaryCoresStarted))) == 0 {
		spinLoopWait()
	}

	schedulerLock.Lock()
	scheduler(false)
	schedulerLock.Unlock()

	for {
		device.Asm("waiti 0")
	}
}

func signalCore(core uint32) {
	if core == 0 {
		esp.SYSTEM.CPU_INTR_FROM_CPU_0.Set(1)
	} else {
		esp.SYSTEM.CPU_INTR_FROM_CPU_1.Set(1)
	}
}

func crossCoreHandler(interrupt.Interrupt) {
	core := currentCPU()

	// The source is level triggered, so clear it before the flags are read.
	if core == 0 {
		esp.SYSTEM.CPU_INTR_FROM_CPU_0.Set(0)
	} else {
		esp.SYSTEM.CPU_INTR_FROM_CPU_1.Set(0)
	}
	device.Asm("memw")

	if gcPauseRequest[core].Swap(0) != 0 {
		gcInterruptHandler(core)
	}
}

// Core in sleepTicksMulticore, or 0xff. Protected by schedulerLock.
var sleepingCore uint8 = 0xff

var sleepInterrupted atomic.Uint32

func hasSleepingCore() bool {
	return sleepingCore != 0xff
}

func sleepTicksMulticore(d timeUnit) {
	core := currentCPU()
	sleepingCore = uint8(core)
	sleepInterrupted.Store(0)
	routeTimerAlarm(core)
	target := ticks() + d

	// waiti enables interrupts and waits in one step, so no wakeup is lost. An
	// interrupt can wake a task before the sleep starts, so check the list.
	mask := interrupt.Disable()
	schedulerLock.Unlock()
	for sleepInterrupted.Load() == 0 && interruptWakes[core] == nil {
		setTimerAlarm(target)
		if ticks() >= target {
			break
		}
		device.Asm("waiti 0")
		interrupt.Disable()
	}
	interrupt.Restore(mask)

	schedulerLock.Lock()
	sleepingCore = 0xff
}

// Send the level triggered TIMG0 alarm only to the given core, so the other
// core cannot clear it first. See INT_MUX_DISABLED_INTNO in ESP-IDF intr_alloc.c.
func routeTimerAlarm(core uint32) {
	const disabled = 6
	if core == 0 {
		esp.INTERRUPT_CORE1.SetTG_T0_INT_MAP(disabled)
		esp.INTERRUPT_CORE0.SetTG_T0_INT_MAP(timerAlarmCPUInterrupt)
	} else {
		esp.INTERRUPT_CORE0.SetTG_T0_INT_MAP(disabled)
		esp.INTERRUPT_CORE1.SetTG_T0_INT_MAP(timerAlarmCPUInterrupt)
	}
}

// Stop sleepTicksMulticore. Must be called with the scheduler lock held.
func interruptSleep() {
	sleepInterrupted.Store(1)
	signalCore(uint32(sleepingCore))
}

// interruptSleepFromInterrupt stops sleepTicksMulticore after an interrupt
// woke a task, so that the scheduler takes it.
func interruptSleepFromInterrupt() {
	sleepInterrupted.Store(1)
}

func interruptSleepTicksMulticore(wakeup timeUnit) {
	if sleepingCore != 0xff {
		interruptSleep()
	}
}

// Cores in schedulerUnlockAndWait. Protected by schedulerLock.
var waitingCores uint8

func schedulerUnlockAndWait() {
	core := currentCPU()
	waitingCores |= 1 << core

	// waiti enables interrupts and waits in one step, so a wakeup that comes
	// after the unlock is not lost. An interrupt can wake a task before that.
	mask := interrupt.Disable()
	schedulerLock.Unlock()
	if interruptWakes[core] == nil {
		device.Asm("waiti 0")
	}
	interrupt.Restore(mask)

	schedulerLock.Lock()
	waitingCores &^= 1 << core
}

func schedulerWake() {
	for core := uint32(0); core < numCPU; core++ {
		if waitingCores&(1<<core) != 0 {
			waitingCores &^= 1 << core
			signalCore(core)
			return
		}
	}
	if sleepingCore != 0xff {
		interruptSleep()
	}
}

func schedulerWakeCore(core uint32) {
	if waitingCores&(1<<core) != 0 {
		waitingCores &^= 1 << core
		signalCore(core)
	} else if uint32(sleepingCore) == core {
		interruptSleep()
	}
}

var (
	gcPauseRequest [numCPU]atomic.Uint32
	gcSignalWait   [numCPU]atomic.Uint32
)

// Stop-the-world handshake with gcMarkReachable and gcResumeWorld.
func gcInterruptHandler(core uint32) {
	gcScanState.Add(1)
	gcWaitForSignal(core)

	scanCurrentStack()
	if !task.OnSystemStack() {
		markRoots(task.SystemStack(), coreStackTop(core))
	}
	gcScanState.Store(1)

	gcWaitForSignal(core)
	gcScanState.Add(1)
}

func gcWaitForSignal(core uint32) {
	for gcSignalWait[core].Load() == 0 {
		spinLoopWait()
	}
	gcSignalWait[core].Store(0)
}

func gcPauseCore(core uint32) {
	gcPauseRequest[core].Store(1)
	signalCore(core)
}

func gcSignalCore(core uint32) {
	gcSignalWait[core].Store(1)
}

func coreStackTop(core uint32) uintptr {
	switch core {
	case 0:
		return uintptr(unsafe.Pointer(&stackTopSymbol))
	case 1:
		return uintptr(unsafe.Pointer(&stack1TopSymbol))
	default:
		runtimeFatal("unexpected core")
		return 0
	}
}

var (
	schedulerLock spinLock
	futexLock     spinLock
	atomicsLock   spinLock
	printLock     spinLock
)

type spinLock struct {
	atomic.Uint32
}

func (l *spinLock) Lock() {
	for !l.Uint32.CompareAndSwap(0, 1) {
		spinLoopWait()
	}
}

func (l *spinLock) Unlock() {
	if schedulerAsserts && l.Uint32.Load() != 1 {
		runtimeFatal("unlock of unlocked spinlock")
	}
	l.Uint32.Store(0)
}

func spinLoopWait() {
}
