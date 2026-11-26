//go:build scheduler.none

package task

// taskExit is tinygo_task_exit, called by tinygo_startTask in
// task_stack_*_windows.S, which every Windows program links. Without a
// scheduler no task starts, but COFF's linker reports an undefined symbol
// even in code it then drops (ELF's doesn't), so it needs a definition. As for
// UEFI, see task_none_uefi.go.
//
//go:export tinygo_task_exit
func taskExit() {
	runtimePanicSchedulerDisabled()
}
