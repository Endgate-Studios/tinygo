package builder

import (
	"strings"
	"testing"
)

func TestInternalUndefined(t *testing.T) {
	in := strings.Join([]string{
		"ld.lld: warning: undefined symbol: memcpy",
		">>> referenced by tls.go:150",
		"ld.lld: warning: undefined symbol: runtime.alloc",
		">>> referenced by main.go:12 (/src/main.go:12)",
		"ld.lld: warning: undefined symbol: tinygo_task_gc_pause",
		">>> referenced by task_threads.c",
		"ld.lld: warning: undefined symbol: getaddrinfo",
		"",
	}, "\r\n")
	want := strings.Join([]string{
		"ld.lld: error: undefined symbol: runtime.alloc",
		">>> referenced by main.go:12 (/src/main.go:12)",
		"ld.lld: error: undefined symbol: tinygo_task_gc_pause",
		">>> referenced by task_threads.c",
		"",
	}, "\n")
	if got := internalUndefined(in); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := internalUndefined("ld.lld: warning: undefined symbol: memset\n"); got != "" {
		t.Errorf("libc only: got %q", got)
	}
	// The kept errors become the same diagnostics as in a program.
	err := parseLLDErrors(internalUndefined(in))
	if err == nil || !strings.Contains(err.Error(), "object allocated on the heap with -gc=none") {
		t.Errorf("parseLLDErrors: %v", err)
	}
}
