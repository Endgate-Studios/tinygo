//go:build numa_psram_octal

package main

// wifiPSRAM is in PSRAM (the .psram section of targets/esp32s3.ld).
//
//go:section .psram
var wifiPSRAM [256 << 10]byte

func psramBuf() []byte { return wifiPSRAM[:] }
