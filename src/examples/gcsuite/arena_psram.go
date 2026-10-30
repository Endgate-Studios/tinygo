//go:build numa_psram_octal

package main

// psramArenaMem is in PSRAM, outside the GC heap and the globals range (targets/esp32s3.ld).
//
//go:section .psram
var psramArenaMem [8 << 10]byte

func psramArena() []byte { return psramArenaMem[:] }
