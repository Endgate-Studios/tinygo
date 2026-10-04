//go:build baremetal

package collector

// At 240 MHz and with the test heap in PSRAM, 20000 steps take minutes. 2000 steps still run many cycles.
func init() { randomSteps = 2000 }
