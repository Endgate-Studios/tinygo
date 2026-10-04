package gcshim

import (
	"runtime"

	"runtime/memory/collector"
)

// Stats is a snapshot of the collector counters and the pause times.
// gcshim measures each cycle, also the cycles that an allocation starts.
type Stats struct {
	collector.Stats
	LastPauseNs  uint64
	MaxPauseNs   uint64
	TotalPauseNs uint64
}

// Runtime gives the program collector as an injectable dependency. It is a memory.Collector,
// and Reserve and Release supply memory for arenas.
type Runtime struct{}

// Collect runs a collection cycle.
func (Runtime) Collect() { runtime.GC() }

// Reserve returns unscanned memory for an arena. See the package function.
func (Runtime) Reserve(n uintptr) []byte { return Reserve(n) }

// Release gives back memory from Reserve.
func (Runtime) Release(mem []byte) { Release(mem) }

// Stats returns the collector counters and the pause times.
func (Runtime) Stats() Stats { return ReadStats() }
