//go:build esp32p4 && numa_psram_hex

package machine

// cpuFrequency is what raiseClock in runtime_esp32p4_psram.go sets, once the runtime has started.
const cpuFrequency = 360e6
