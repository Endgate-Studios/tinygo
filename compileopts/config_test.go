package compileopts

import (
	"slices"
	"testing"
)

func TestExtraFilesBoehm(t *testing.T) {
	target := &TargetSpec{
		GC: "precise",
		ExtraFiles: []string{
			"src/runtime/asm_tinygowasm.S",
		},
	}
	config := &Config{
		Options: &Options{},
		Target:  target,
	}

	got := config.ExtraFiles()
	want := []string{"src/runtime/asm_tinygowasm.S"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected precise GC files: got %v, want %v", got, want)
	}

	config.Options.GC = "boehm"
	got = config.ExtraFiles()
	want = []string{
		"src/runtime/asm_tinygowasm.S",
		"src/runtime/gc_boehm.c",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected Boehm GC files: got %v, want %v", got, want)
	}
}

// A native c-shared is a shared library; wasm-unknown says GOOS linux, but
// its c-shared is a wasm module.
func TestSharedLibrary(t *testing.T) {
	for _, tc := range []struct {
		triple, goos, buildMode string
		want                    bool
	}{
		{"x86_64-unknown-linux-musl", "linux", "c-shared", true},
		{"x86_64-unknown-linux-musl", "linux", "default", false},
		{"x86_64-unknown-windows-gnu", "windows", "c-shared", true},
		{"wasm32-unknown-unknown", "linux", "c-shared", false},
		{"wasm32-unknown-wasi", "wasip1", "c-shared", false},
	} {
		config := &Config{
			Options: &Options{BuildMode: tc.buildMode},
			Target:  &TargetSpec{Triple: tc.triple, GOOS: tc.goos},
		}
		if got := config.SharedLibrary(); got != tc.want {
			t.Errorf("%s %s %s: SharedLibrary() = %v, want %v", tc.triple, tc.goos, tc.buildMode, got, tc.want)
		}
	}
}
