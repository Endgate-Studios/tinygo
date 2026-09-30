//go:build (scheduler.tasks || scheduler.cores) && !tinygo.riscv && !(scheduler.cores && esp32s3)

package task

// OnSystemStack returns whether the caller is running on the system stack.
func OnSystemStack() bool {
	// If there is no active goroutine, this must be the system stack.
	return Current() == nil
}
