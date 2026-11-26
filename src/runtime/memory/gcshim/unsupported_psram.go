//go:build gc.custom && gcshim_psram && !(baremetal && esp32s3 && numa_psram_octal)

package gcshim

// The gcshim_psram tag needs an ESP32-S3 Octal PSRAM target (targets/esp32s3-psram-octal.json).
// NOTES.md (PSRAM in the GC heap) gives the details.
var _ = gcshim_psram_needs_esp32s3_numa_psram_octal
