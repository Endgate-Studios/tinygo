//go:build esp32p4 && !numa_psram_hex

package runtime

func initPSRAM() {}

// PSRAMTuning is how the PSRAM was started, which with no PSRAM is not at all.
type PSRAMTuning struct {
	MHz        uint32
	Phase      uint8
	Delay      uint8
	Passing    uint8
	TestFailed bool
}

// PSRAMState is what the PSRAM was started with: nothing, in a build without it.
func PSRAMState() PSRAMTuning { return PSRAMTuning{} }
