//go:build esp32p4 && !numa_psram_hex

package runtime

func initPSRAM() {}
