//go:build esp32s3 && !scheduler.cores

package radio

import "unsafe"

//go:linkname currentTask internal/task.Current
func currentTask() unsafe.Pointer
