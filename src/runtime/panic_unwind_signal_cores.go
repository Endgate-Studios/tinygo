//go:build (tinygo.unwind.explicit || tinygo.unwind.asyncify) && scheduler.cores

package runtime

var unwindPendingSignal [numCPU]bool

// The index can't be out of range, so these have no bounds check and can't
// panic: with asyncify, a function that can panic is instrumented, and
// unwindPending is called while a panic replay rewinds.

//go:inline
//go:nobounds
func getUnwindSignal() bool {
	return unwindPendingSignal[currentCPU()]
}

//go:inline
//go:nobounds
func setUnwindSignal(unwinding bool) {
	unwindPendingSignal[currentCPU()] = unwinding
}
