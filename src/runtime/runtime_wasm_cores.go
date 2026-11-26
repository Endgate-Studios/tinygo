//go:build scheduler.cores && tinygo.wasm.cores

package runtime

import (
	"internal/gclayout"
	"internal/task"
	"sync/atomic"
	"unsafe"
)

// The cores are Web Workers on one shared memory (targets/wasm_exec_cores.js).
// Core 0 is the page's own thread, which may not block: it starts the
// others and goes back to the event loop, and the goroutines, main among
// them, run on cores 1 and up.
const numCPU = 8

var (
	printLock     spinLock
	schedulerLock spinLock
	atomicsLock   spinLock
	futexLock     spinLock
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

// spinLoopWait is where a core takes a pause request: wasm has no interrupt
// between cores, so a core spinning or waiting (locks, the heap) checks.
func spinLoopWait() {
	core := currentCPU()
	if atomic.SwapUint32(&gcPauseRequest[core], 0) != 0 {
		gcInterruptHandler(core)
	}
}

//go:threadlocal
var coreID uint32

func currentCPU() uint32 {
	return coreID
}

//export tinygo_atomic_wait32
func atomicWait32(addr *uint32, expected uint32, timeout int64) int32

//export tinygo_atomic_notify
func atomicNotify(addr *uint32, count uint32) uint32

//go:wasmimport tinygo_cores start_core
func startCore(core uint32, stackTop, tls uintptr)

//go:wasmimport tinygo_cores core_count
func jsCoreCount() uint32

// exitWord is the program's end: 0 while it runs, its exit code plus one
// after. Core 0's page waits on it (Atomics.waitAsync).
var exitWord uint32

//export tinygo_cores_exit_word
func coresExitWord() uintptr {
	return uintptr(unsafe.Pointer(&exitWord))
}

// coreStackSize is each secondary core's system stack.
const coreStackSize = 256 << 10

var (
	coresRunning         uint32 // cores started, core 0 counted
	coreStacks           [numCPU]uintptr
	coreBlocks           [numCPU][2]unsafe.Pointer // each core's stack and TLS block, kept alive
	schedulerWakePending [numCPU]uint32
	sleepWake            [numCPU]uint32
	// coreWake is what an idle core waits on: bumped by whatever wakes it.
	coreWake [numCPU]uint32
)

func run() {
	initRand()
	initHeap()
	go func() {
		initAll()
		callMain()
		mainExited.Store(1)
		endProgram(0)
	}()
	secondaryCoresReady.Store(1)
	startSecondaryCores()
}

// endProgram tells the page the program's ended and every core to stop.
func endProgram(code int) {
	atomic.StoreUint32(&exitWord, uint32(code)+1)
	atomicNotify(&exitWord, ^uint32(0))
	mainExited.Store(1)
	for core := uint32(1); core < atomic.LoadUint32(&coresRunning); core++ {
		atomic.StoreUint32(&schedulerWakePending[core], 1)
		atomic.StoreUint32(&sleepWake[core], 1)
		wakeCore(core)
	}
}

func startSecondaryCores() {
	n := jsCoreCount()
	if n > numCPU {
		n = numCPU
	}
	if n < 2 {
		n = 2
	}
	// Every core's blocks first: a core started runs goroutines at once.
	for core := uint32(1); core < n; core++ {
		// The stack is scanned by its live range, the TLS block whole.
		stack := alloc(coreStackSize, gclayout.NoPtrs.AsPtr())
		coreStacks[core] = (uintptr(stack) + coreStackSize) &^ 15
		coreBlocks[core] = [2]unsafe.Pointer{stack, alloc(tlsBlockSize, gclayout.Conservative.AsPtr())}
	}
	// Counted only now: a collection while the blocks were made had no
	// other core to wait for.
	atomic.StoreUint32(&coresRunning, n)
	for core := uint32(1); core < n; core++ {
		startCore(core, coreStacks[core], (uintptr(coreBlocks[core][1])+15)&^15)
	}
}

// tlsBlockSize is room for a core's thread-locals (wasm-ld's __tls_size,
// rounded up, with room to align).
const tlsBlockSize = 1024

//export tinygo_core_entry
func runSecondary(core uint32) {
	coreID = core
	waitForSecondaryCoresReady()
	schedulerLock.Lock()
	scheduler(false)
	schedulerLock.Unlock()
}

var waitingCores uint32

func schedulerUnlockAndWait() {
	core := currentCPU()
	waitingCores |= 1 << core
	schedulerLock.Unlock()
	for {
		seq := atomic.LoadUint32(&coreWake[core])
		spinLoopWait()
		if atomic.SwapUint32(&schedulerWakePending[core], 0) != 0 || interruptWakes[core] != nil || mainExited.Load() != 0 {
			break
		}
		atomicWait32(&coreWake[core], seq, -1)
	}
	schedulerLock.Lock()
	waitingCores &^= 1 << core
}

func schedulerWake() {
	for core := uint32(1); core < numCPU; core++ {
		if waitingCores&(1<<core) != 0 {
			schedulerWakeCore(core)
			return
		}
	}
	if hasSleepingCore() {
		interruptSleepTicksMulticore(0)
	}
}

func schedulerWakeCore(core uint32) {
	if waitingCores&(1<<core) != 0 {
		waitingCores &^= 1 << core
		atomic.StoreUint32(&schedulerWakePending[core], 1)
		wakeCore(core)
	} else if uint32(sleepingCore) == core {
		interruptSleepTicksMulticore(0)
	}
}

var sleepingCore uint8 = 0xff

func hasSleepingCore() bool {
	return sleepingCore != 0xff
}

func sleepTicksMulticore(d timeUnit) {
	core := currentCPU()
	sleepingCore = uint8(core)
	atomic.StoreUint32(&sleepWake[core], 0)
	schedulerLock.Unlock()
	target := ticks() + d
	for {
		seq := atomic.LoadUint32(&coreWake[core])
		spinLoopWait()
		now := ticks()
		if atomic.LoadUint32(&sleepWake[core]) != 0 || now >= target {
			break
		}
		atomicWait32(&coreWake[core], seq, ticksToNanoseconds(target-now))
	}
	schedulerLock.Lock()
	sleepingCore = 0xff
}

func interruptSleepTicksMulticore(wakeup timeUnit) {
	if hasSleepingCore() {
		core := uint32(sleepingCore)
		atomic.StoreUint32(&sleepWake[core], 1)
		wakeCore(core)
	}
}

func interruptSleepFromInterrupt() {
	core := currentCPU()
	atomic.StoreUint32(&sleepWake[core], 1)
	atomic.StoreUint32(&schedulerWakePending[core], 1)
}

// exitGoroutine ends a goroutine that returned from its function.
func exitGoroutine() {
	task.Exit()
	runtimeFatal("unreachable")
}

// NumCPU is the cores that run goroutines: every core but the page's own.
func NumCPU() int {
	n := int(atomic.LoadUint32(&coresRunning)) - 1
	if n < 1 {
		return 1
	}
	return n
}

// wakeCore ends an idle core's wait, to look at what changed.
func wakeCore(core uint32) {
	atomic.AddUint32(&coreWake[core], 1)
	atomicNotify(&coreWake[core], 1)
}

func signalCore(core uint32) {
	wakeCore(core)
}

// The stop-the-world of gc_stack_cores.go. A core takes the pause request in
// spinLoopWait; one that runs no Go (the page, a core not started) answers
// for itself at once, with no stack of its own to scan.
var (
	gcPauseRequest [numCPU]uint32
	gcSignalWait   [numCPU]uint32
	gcStandIn      [numCPU]uint8
)

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
	for atomic.SwapUint32(&gcSignalWait[core], 0) == 0 {
		atomicWait32(&gcSignalWait[core], 0, -1)
	}
}

// runsGo is whether a core runs goroutines: cores 1 and up, once started.
func runsGo(core uint32) bool {
	return core != 0 && core < atomic.LoadUint32(&coresRunning)
}

func gcPauseCore(core uint32) {
	if !runsGo(core) {
		gcStandIn[core] = 1
		gcScanState.Add(1)
		return
	}
	atomic.StoreUint32(&gcPauseRequest[core], 1)
	signalCore(core)
}

func gcSignalCore(core uint32) {
	switch gcStandIn[core] {
	case 1: // scan: nothing of its own
		gcStandIn[core] = 2
		gcScanState.Store(1)
	case 2: // resume
		gcStandIn[core] = 0
		gcScanState.Add(1)
	default:
		atomic.StoreUint32(&gcSignalWait[core], 1)
		atomicNotify(&gcSignalWait[core], 1)
	}
}

func coreStackTop(core uint32) uintptr {
	if core == 0 {
		return stackTop
	}
	return coreStacks[core]
}
