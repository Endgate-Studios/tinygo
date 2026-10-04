//go:build gc.custom && !(tinygo.wasm || (baremetal && (esp32 || esp32s3 || esp32c3 || scheduler.cores)))

package gcshim

// Other targets fail the build here with a readable name.
// NOTES.md (Target support) gives the reasons.
var _ = gcshim_supports_wasm_single_core_esp32_or_scheduler_cores
