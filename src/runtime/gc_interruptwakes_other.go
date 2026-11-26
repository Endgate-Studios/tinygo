//go:build !scheduler.cores

package runtime

// Only the cores scheduler has interrupt wake lists, see scheduler_cores.go.
func gcMarkInterruptWakes() (heads [0]uintptr) { return }

func interruptWakesChanged(heads [0]uintptr) bool { return false }

func hasInterruptWakes(core uint32) bool { return false }
