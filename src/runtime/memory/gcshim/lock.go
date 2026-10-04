//go:build gc.custom && !scheduler.cores

package gcshim

// heapLock does nothing without scheduler.cores. wasm has one thread and the tasks scheduler
// is cooperative (src/runtime/scheduler_cooperative.go), so nothing interrupts an allocation.
type heapLock struct{}

func (*heapLock) Lock()   {}
func (*heapLock) Unlock() {}
