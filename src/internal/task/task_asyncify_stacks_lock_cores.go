//go:build scheduler.cores && tinygo.wasm

package task

import "sync/atomic"

// stacksLock guards the list of task stacks, which every core links into.
var stacksLock atomic.Uint32

func lockStacks() {
	for !stacksLock.CompareAndSwap(0, 1) {
	}
}

func unlockStacks() {
	stacksLock.Store(0)
}
