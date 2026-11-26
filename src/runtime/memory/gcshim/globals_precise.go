//go:build gc.custom && (gcshim_precise_globals || windows || tinygo.wasm)

package gcshim

// runtimeMarkGlobals scans only the pointer slots of globals. It needs TinyGo 0.43 or
// later (src/runtime/gc_globals_custom.go, tinygo commit 59fb1040). On wasm, scanning
// every word read text and numbers in globals as pointers into big objects.
//
//go:linkname runtimeMarkGlobals runtime.markGlobals
func runtimeMarkGlobals()

func markGlobals() {
	runtimeMarkGlobals()
}
