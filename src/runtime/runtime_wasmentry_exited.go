//go:build tinygo.wasm && !scheduler.cores

package runtime

func hasMainExited() bool { return mainExited }
