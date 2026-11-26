//go:build windows

package main

import (
	"syscall"
	"testing"

	"github.com/tinygo-org/tinygo/builder"
)

// A Windows DLL links (the runtime's exported functions stay visible to its
// own assembly), loads, and calls into Go.
func TestWindowsCSharedDLL(t *testing.T) {
	t.Parallel()
	options := optionsFromTarget("", sema)
	options.BuildMode = "c-shared"
	options.Scheduler = "none"
	options.GC = "leaking"
	config, err := builder.NewConfig(&options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := builder.Build("./testdata/cshared-windows", ".dll", t.TempDir(), config)
	if err != nil {
		t.Fatal("failed to build the DLL:", err)
	}
	dll, err := syscall.LoadDLL(result.Binary)
	if err != nil {
		t.Fatal(err)
	}
	defer dll.Release()
	if _, _, err := dll.MustFindProc("tinygo_init").Call(); err != syscall.Errno(0) {
		t.Fatal("tinygo_init:", err)
	}
	r, _, _ := dll.MustFindProc("add").Call(2, 3)
	if int32(r) != 5 {
		t.Errorf("add(2, 3) = %d, want 5", int32(r))
	}
}
