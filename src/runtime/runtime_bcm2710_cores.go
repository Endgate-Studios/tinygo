//go:build bcm2710 && scheduler.cores

package runtime

import (
	"device/arm64"
	"device/bcm2710"
	"internal/task"
	"runtime/interrupt"
	"sync/atomic"
	"unsafe"
)

const numCPU = 4

var (
	printLock     spinLock
	schedulerLock spinLock
	atomicsLock   spinLock
	futexLock     spinLock
)

// spinLock uses the A53's exclusive loads and stores. The armstub sets CPUECTLR.SMPEN on
// every core, and start.S maps RAM inner shareable, so the cores see one lock.
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
	arm64.Asm("yield")
}

func currentCPU() uint32 {
	return uint32(arm64.AsmFull("mrs {}, mpidr_el1", nil)) & 3
}

var (
	coresStarted         atomic.Uint32
	schedulerWakePending [numCPU]atomic.Uint32
	sleepWake            [numCPU]atomic.Uint32
	gcPauseRequest       [numCPU]atomic.Uint32
	gcSignalWait         [numCPU]atomic.Uint32
)

// signalCore sets a bit in the core's mailbox 0, which interrupts it.
func signalCore(core uint32) {
	bcm2710.LOCAL.MAILBOX_SET[core*4].Set(1)
}

// bcm2710CrossCore runs in a core's mailbox 0 interrupt.
func bcm2710CrossCore(core uint32) {
	if gcPauseRequest[core].Swap(0) != 0 {
		gcInterruptHandler(core)
	}
}

//go:extern tinygo_secondaryStart
var secondaryStart [0]byte

// startSecondaryCores writes the entry address to the armstub's spin table, cleans it to
// memory as the waiting cores run with caches off, and wakes them with sev
// (raspberrypi/tools armstubs/armstub8.S, spin_cpu1 to spin_cpu3).
func startSecondaryCores() {
	entry := uint64(uintptr(unsafe.Pointer(&secondaryStart)))
	for core := uint32(1); core < numCPU; core++ {
		enableCoreInterrupts(core)
		slot := uintptr(0xd8 + 8*core)
		*(*uint64)(unsafe.Pointer(slot)) = entry
		arm64.AsmFull("dc civac, {a}", map[string]interface{}{"a": slot})
	}
	arm64.Asm("dsb sy")
	arm64.Asm("sev")
	for coresStarted.Load() != numCPU-1 {
		spinLoopWait()
	}
}

//export tinygo_runSecondary
func runSecondary(core uint32) {
	arm64.Asm("msr daifclr, #2")
	coresStarted.Add(1)

	waitForSecondaryCoresReady()

	schedulerLock.Lock()
	scheduler(false)
	schedulerLock.Unlock()

	exit(0)
}

// Bitset of cores waiting in schedulerUnlockAndWait. Accessed with the scheduler lock held.
var waitingCores uint8

// schedulerUnlockAndWait sleeps the core until another wakes it. Call with the scheduler lock held.
func schedulerUnlockAndWait() {
	core := currentCPU()
	waitingCores |= 1 << core

	state := interrupt.Disable()
	schedulerLock.Unlock()
	for schedulerWakePending[core].Swap(0) == 0 && interruptWakes[core] == nil {
		waitForInterrupt()
	}
	interrupt.Restore(state)
	schedulerLock.Lock()
	waitingCores &^= 1 << core
}

// schedulerWake wakes one waiting or sleeping core. Call with the scheduler lock held.
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

// schedulerWakeCore wakes the core if it's waiting or sleeping. Call with the scheduler lock held.
func schedulerWakeCore(core uint32) {
	if waitingCores&(1<<core) != 0 {
		waitingCores &^= 1 << core
		schedulerWakePending[core].Store(1)
		signalCore(core)
	} else if uint32(sleepingCore) == core {
		interruptSleepTicksMulticore(0)
	}
}

// The core that sleeps on its timer, or 0xff. Accessed with the scheduler lock held.
var sleepingCore uint8 = 0xff

func hasSleepingCore() bool {
	return sleepingCore != 0xff
}

// sleepTicksMulticore sleeps on this core's timer with the scheduler unlocked, until the time
// or interruptSleepTicksMulticore. Call with the scheduler lock held.
func sleepTicksMulticore(d timeUnit) {
	core := currentCPU()
	sleepingCore = uint8(core)
	sleepWake[core].Store(0)
	target := ticks() + d

	mask := interrupt.Disable()
	schedulerLock.Unlock()
	for sleepWake[core].Load() == 0 && interruptWakes[core] == nil {
		if ticks() >= target {
			break
		}
		setTimer(target)
		waitForInterrupt()
	}
	interrupt.Restore(mask)

	schedulerLock.Lock()
	sleepingCore = 0xff
}

// interruptSleepTicksMulticore stops the sleep of the sleeping core. Call with the scheduler lock held.
func interruptSleepTicksMulticore(wakeup timeUnit) {
	if hasSleepingCore() {
		sleepWake[sleepingCore].Store(1)
		signalCore(uint32(sleepingCore))
	}
}

// interruptSleepFromInterrupt ends this core's wait after an interrupt woke a task.
func interruptSleepFromInterrupt() {
	core := currentCPU()
	sleepWake[core].Store(1)
	schedulerWakePending[core].Store(1)
}

// gcInterruptHandler is the stop-the-world pause of a core the GC interrupted.
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
	signalCore(core)
}

func gcSignalCore(core uint32) {
	gcSignalWait[core].Store(1)
}

//go:extern _stack1_top
var stack1TopSymbol [0]byte

//go:extern _stack2_top
var stack2TopSymbol [0]byte

//go:extern _stack3_top
var stack3TopSymbol [0]byte

// coreStackTop returns the top of a core's system stack.
func coreStackTop(core uint32) uintptr {
	switch core {
	case 0:
		return uintptr(unsafe.Pointer(&stackTopSymbol))
	case 1:
		return uintptr(unsafe.Pointer(&stack1TopSymbol))
	case 2:
		return uintptr(unsafe.Pointer(&stack2TopSymbol))
	case 3:
		return uintptr(unsafe.Pointer(&stack3TopSymbol))
	}
	runtimeFatal("unexpected core")
	return 0
}
