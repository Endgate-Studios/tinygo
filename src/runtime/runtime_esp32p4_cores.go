//go:build esp32p4 && scheduler.cores

package runtime

import (
	"device/esp"
	"device/riscv"
	"internal/task"
	"runtime/interrupt"
	"runtime/volatile"
	"sync/atomic"
	"unsafe"
)

const numCPU = 2

// The FROM_CPU software interrupts use this CPU interrupt on both cores.
const crossCoreCPUInterrupt = 3

var (
	printLock     spinLock
	schedulerLock spinLock
	atomicsLock   spinLock
	futexLock     spinLock
	ticksLock     spinLock
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
	riscv.Asm("nop")
}

func currentCPU() uint32 {
	return uint32(riscv.MHARTID.Get())
}

func lockTicks() interrupt.State {
	mask := interrupt.Disable()
	ticksLock.Lock()
	return mask
}

func unlockTicks(mask interrupt.State) {
	ticksLock.Unlock()
	interrupt.Restore(mask)
}

var crossCoreInterrupt interrupt.Interrupt

var (
	core1Started         atomic.Uint32
	schedulerWakePending [numCPU]atomic.Uint32
	sleepWake            [numCPU]atomic.Uint32
	gcPauseRequest       [numCPU]atomic.Uint32
	gcSignalWait         [numCPU]atomic.Uint32
	core0EnableRequest   atomic.Uint32
)

func signalCore(core, value uint32) {
	if core == 0 {
		esp.HP_SYS.CPU_INTR_FROM_CPU_0.Set(value)
	} else {
		esp.HP_SYS.CPU_INTR_FROM_CPU_1.Set(value)
	}
}

func crossCoreHandler(interrupt.Interrupt) {
	core := currentCPU()

	// The source is level triggered, so clear it before reading the flags.
	signalCore(core, 0)
	riscv.Asm("fence")
	if core == 0 {
		if mask := core0EnableRequest.Load(); mask != 0 {
			interruptEnableLocal(mask)
			core0EnableRequest.And(^mask)
		}
	}
	if gcPauseRequest[core].Swap(0) != 0 {
		gcInterruptHandler(core)
	}
	if flashPauseRequest[core].Swap(0) != 0 {
		tinygo_flashPark(&flashParkState)
	}
}

// Values of flashParkState, shared with tinygo_flashPark in esp32p4.S.
const (
	flashParkIdle    = 0
	flashParkLocked  = 1
	flashParkRelease = 2
)

var (
	flashSafeLock     spinLock
	flashPauseRequest [numCPU]atomic.Uint32
	flashParkState    uint32
)

//export tinygo_flashPark
func tinygo_flashPark(state *uint32)

// esp32p4EnterFlashSafeSection parks the other core in RAM before a flash erase or write. It must not be
// called from an interrupt or with interrupts disabled. Interrupts go off first: a GC pause here would wait
// for the other core while that waits in RAM.
func esp32p4EnterFlashSafeSection() (interrupt.State, bool) {
	if secondaryCoresReady.Load() == 0 {
		return interrupt.Disable(), false
	}
	flashSafeLock.Lock()
	state := interrupt.Disable()
	other := 1 - currentCPU()
	flashPauseRequest[other].Store(1)
	signalCore(other, 1)
	for volatile.LoadUint32(&flashParkState) != flashParkLocked {
		spinLoopWait()
	}
	return state, true
}

func esp32p4ExitFlashSafeSection(state interrupt.State, multicore bool) {
	if multicore {
		volatile.StoreUint32(&flashParkState, flashParkRelease)
		for volatile.LoadUint32(&flashParkState) != flashParkIdle {
			spinLoopWait()
		}
		flashSafeLock.Unlock()
	}
	interrupt.Restore(state)
}

// enableInterruptOnCore0 enables a CPU interrupt on core 0, where the interrupt matrix sends them.
func enableInterruptOnCore0(num uint32) {
	bit := uint32(1) << num
	core0EnableRequest.Or(bit)
	signalCore(0, 1)
	for core0EnableRequest.Load()&bit != 0 {
		spinLoopWait()
	}
}

//go:linkname interruptEnableLocal runtime/interrupt.enableLocal
func interruptEnableLocal(mask uint32)

// startSecondaryCores follows start_other_core in ESP-IDF components/esp_system/port/cpu_start.c.
func startSecondaryCores() {
	esp.INTERRUPT_CORE0.CPU_INT_FROM_CPU_0_MAP.Set(crossCoreCPUInterrupt + clicExternalStart)
	esp.INTERRUPT_CORE1.CPU_INT_FROM_CPU_1_MAP.Set(crossCoreCPUInterrupt + clicExternalStart)
	signalCore(0, 0)
	signalCore(1, 0)
	crossCoreInterrupt = interrupt.New(crossCoreCPUInterrupt, crossCoreHandler)
	crossCoreInterrupt.Enable()

	// Core 1 has its own L1 instruction cache, and runs from flash.
	// See cache_ll_l1_enable_icache in ESP-IDF hal/esp32p4/include/hal/cache_ll.h.
	Cache_Enable_L1_CORE1_ICache(0)

	esp.HP_SYS_CLKRST.SetSOC_CLK_CTRL0_REG_CORE1_CPU_CLK_EN(1)
	esp.HP_SYS_CLKRST.SetHP_RST_EN0_REG_RST_EN_CORE1_GLOBAL(0)
	ets_set_appcpu_boot_addr(uintptr(unsafe.Pointer(&call_start_cpu1)))

	for core1Started.Load() == 0 {
		spinLoopWait()
	}
}

//export Cache_Enable_L1_CORE1_ICache
func Cache_Enable_L1_CORE1_ICache(autoload uint32) uint32

//export ets_set_appcpu_boot_addr
func ets_set_appcpu_boot_addr(addr uintptr)

//go:extern call_start_cpu1
var call_start_cpu1 [0]byte

// The function that is started on the second core, from call_start_cpu1.
//
//export tinygo_runCore1
func runCore1() {
	interruptInit()
	ets_set_appcpu_boot_addr(0)

	interruptEnableLocal(1 << crossCoreCPUInterrupt)
	core1Started.Store(1)

	waitForSecondaryCoresReady()

	schedulerLock.Lock()
	scheduler(false)
	schedulerLock.Unlock()

	exit(0)
}

// Bitset of cores waiting in schedulerUnlockAndWait.
// Must only be accessed with the scheduler lock held.
var waitingCores uint8

// schedulerUnlockAndWait waits by polling, with interrupts on so the GC can pause this core.
func schedulerUnlockAndWait() {
	core := currentCPU()
	waitingCores |= 1 << core
	schedulerLock.Unlock()
	for schedulerWakePending[core].Swap(0) == 0 && interruptWakes[core] == nil {
		spinLoopWait()
	}
	schedulerLock.Lock()
	waitingCores &^= 1 << core
}

// Wake another core, if one is waiting or sleeping. Must be called with the
// scheduler lock held.
func schedulerWake() {
	for core := uint32(0); core < numCPU; core++ {
		if waitingCores&(1<<core) != 0 {
			schedulerWakeCore(core)
			return
		}
	}
	if hasSleepingCore() {
		interruptSleepTicksMulticore(0)
	}
}

// Wake the given core if it is waiting or sleeping. Must be called with the
// scheduler lock held.
func schedulerWakeCore(core uint32) {
	if waitingCores&(1<<core) != 0 {
		waitingCores &^= 1 << core
		schedulerWakePending[core].Store(1)
	} else if uint32(sleepingCore) == core {
		interruptSleepTicksMulticore(0)
	}
}

// Currently sleeping core, or 0xff.
// Must only be accessed with the scheduler lock held.
var sleepingCore uint8 = 0xff

func hasSleepingCore() bool {
	return sleepingCore != 0xff
}

// sleepTicksMulticore is sleepTicks with the scheduler unlocked, and interruptSleepTicksMulticore
// can stop it. Call with the scheduler lock held.
func sleepTicksMulticore(d timeUnit) {
	core := currentCPU()
	sleepingCore = uint8(core)
	sleepWake[core].Store(0)
	target := ticks() + d
	schedulerLock.Unlock()
	for sleepWake[core].Load() == 0 && interruptWakes[core] == nil && ticks() < target {
		spinLoopWait()
	}
	schedulerLock.Lock()
	sleepingCore = 0xff
}

// Interrupt an ongoing call to sleepTicksMulticore on another core.
// Must be called with the scheduler lock held.
func interruptSleepTicksMulticore(wakeup timeUnit) {
	if hasSleepingCore() {
		sleepWake[sleepingCore].Store(1)
	}
}

// interruptSleepFromInterrupt stops the wait or sleep of this core after an
// interrupt woke a task, so that the scheduler takes it.
func interruptSleepFromInterrupt() {
	core := currentCPU()
	sleepWake[core].Store(1)
	schedulerWakePending[core].Store(1)
}

// gcInterruptHandler holds this core for the stop-the-world phase, and scans its stacks.
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
	for gcSignalWait[core].Swap(0) == 0 {
		spinLoopWait()
	}
}

func gcPauseCore(core uint32) {
	gcPauseRequest[core].Store(1)
	signalCore(core, 1)
}

func gcSignalCore(core uint32) {
	gcSignalWait[core].Store(1)
}

//go:extern _stack1_top
var stack1TopSymbol [0]byte

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
