//go:build gc.custom && baremetal && scheduler.cores

package gcshim

// Multicore root scan uses the runtime stop-the-world in src/runtime/gc_stack_cores.go.

//go:linkname gcMarkReachable runtime.gcMarkReachable
func gcMarkReachable()

//go:linkname gcResumeWorld runtime.gcResumeWorld
func gcResumeWorld()

// markCurrentGoroutineStack is the hook in src/runtime/gc_custom.go. sp is in
// a goroutine stack, which is a heap object. The mark keeps it and scans only its live part above sp.
//
//go:linkname markCurrentGoroutineStack runtime.markCurrentGoroutineStack
func markCurrentGoroutineStack(sp uintptr) {
	shim.heap.MarkStack(sp)
}

// markAllRoots stops the other cores and scans all stacks and the globals.
// The other cores stay stopped until resumeWorld.
func markAllRoots() {
	gcMarkReachable()
}

func resumeWorld() {
	gcResumeWorld()
}
