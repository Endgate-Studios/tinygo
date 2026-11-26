//go:build !tinygo.unicore && !(scheduler.cores && tinygo.wasm)

package task

func cannotPause() bool { return false }

// spinLoopWait is never reached: a goroutine can always pause.
func spinLoopWait() {}
