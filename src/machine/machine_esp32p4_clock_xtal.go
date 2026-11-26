//go:build esp32p4 && !numa_psram_hex

package machine

// cpuFrequency is the 40 MHz crystal, which the runtime selects at startup.
const cpuFrequency = 40e6
