//go:build (scheduler.asyncify || scheduler.cores) && tinygo.wasm.cores

package task

// One of each per core.

// currentTask is the current running task, or nil if currently in the scheduler.
//
//go:threadlocal
var currentTask *Task

// Panic unwinding is synchronous: it either reaches a defer frame or aborts
// without switching tasks. A single transient header is therefore sufficient.
//
//go:threadlocal
var panicStackState stackState
