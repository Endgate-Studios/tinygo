//go:build tinygo.wasm && scheduler.cores

package runtime

// On the cores main ends on another core: the page learns of it by the
// exit word, not by _start returning.
func hasMainExited() bool { return false }
