//go:build !scheduler.cores

package runtime

import "internal/task"

type timerLock = task.PMutex
type chanLock = task.PMutex
type synctestLock = task.PMutex
