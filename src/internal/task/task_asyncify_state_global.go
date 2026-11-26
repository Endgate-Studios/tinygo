//go:build scheduler.asyncify && !tinygo.wasm.cores

package task

// currentTask is the current running task, or nil if currently in the scheduler.
var currentTask *Task

// Panic unwinding is synchronous: it either reaches a defer frame or aborts
// without switching tasks. A single transient header is therefore sufficient.
var panicStackState stackState
