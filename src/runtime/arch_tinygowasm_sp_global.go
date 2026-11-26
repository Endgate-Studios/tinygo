//go:build tinygo.wasm && !scheduler.cores

package runtime

// savedStackPointer is used to save the system stack pointer while in a goroutine.
var savedStackPointer uintptr

// saveStackPointer is called by internal/task.state.Resume() to save the stack pointer before entering a goroutine from the system stack.
func saveStackPointer() {
	savedStackPointer = getCurrentStackPointer()
}
