//go:build esp32s3 && scheduler.cores

package radio

import "unsafe"

// With the cores scheduler, internal/task.Current is itself a link name of
// runtime.currentTask (src/internal/task/task_stack_multicore.go).
//
//go:linkname currentTask runtime.currentTask
func currentTask() unsafe.Pointer
