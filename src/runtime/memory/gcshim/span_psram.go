//go:build gc.custom && baremetal && esp32s3 && numa_psram_octal && gcshim_psram

package gcshim

import (
	"runtime/volatile"
	"unsafe"

	"runtime/memory/collector"
)

// gcHeap is internal DRAM first and mapped PSRAM second. NOTES.md (PSRAM in the GC heap) has the risks.
type gcHeap = collector.Set

// allocFirst allocates from internal DRAM only.
func allocFirst(size uintptr, noPointers bool) unsafe.Pointer {
	if noPointers {
		return shim.heap.AllocPrimaryBytes(size)
	}
	return shim.heap.AllocPrimary(size)
}

// heapSpans holds the two collectors. It is in shim, so markRoots skips their counters.
type heapSpans struct {
	dram, psram collector.Collector
}

const (
	// Larger pages make the PSRAM metadata smaller. Small objects go to DRAM first.
	psramPageSize = 4096

	// MMU table facts from ESP-IDF components/soc/esp32s3/include/soc/ext_mem_defs.h
	// (DR_REG_MMU_TABLE, SOC_MMU_INVALID, SOC_MMU_ACCESS_SPIRAM, SOC_MMU_VALID_VAL_MASK).
	mmuTable      = 0x600C5000
	mmuInvalid    = 1 << 14
	mmuSpiram     = 1 << 15
	mmuPhysMask   = 0x3FFF
	mmuLinearMask = 0x1FFFFFF
	mmuPageSize   = 64 << 10
)

// These symbols come from targets/esp32s3.ld.
//
//go:extern _epsram
var psramDataEndSymbol [0]byte

//go:extern _psram_start
var psramStartSymbol [0]byte

//go:extern _psram_end
var psramEndSymbol [0]byte

// psramInitError is not empty when the PSRAM bring-up failed (src/runtime/runtime_esp32_psram.go).
//
//go:extern runtime.psramInitError
var psramInitError string

// initHeapSpans starts the DRAM span. It adds the PSRAM span after .psram only if PSRAM is mapped.
func initHeapSpans(start, end uintptr) error {
	if err := shim.spans.dram.Init(start, end, end, collectorConfig()); err != nil {
		return err
	}
	s, err := collector.NewSet(&shim.spans.dram, initPSRAMSpan())
	if err != nil {
		return err
	}
	shim.heap = s
	return nil
}

// initPSRAMSpan returns nil if no PSRAM is mapped after the .psram section.
func initPSRAMSpan() *collector.Collector {
	start := (uintptr(unsafe.Pointer(&psramDataEndSymbol)) + 15) &^ 15
	end := psramMappedEnd()
	cfg := collectorConfig()
	cfg.PageSize = psramPageSize
	if end <= start || shim.spans.psram.Init(start, end, end, cfg) != nil {
		return nil
	}
	return &shim.spans.psram
}

// psramMappedEnd counts the MMU entries that initPSRAM mapped. It runs before initHeap
// (src/runtime/runtime_esp32s3.go) and maps page i of the window to PSRAM page i.
// The boot code set all other entries invalid (Cache_MMU_Init in src/device/esp/esp32s3.S).
func psramMappedEnd() uintptr {
	start := uintptr(unsafe.Pointer(&psramStartSymbol))
	if psramInitError != "" {
		return start
	}
	end := uintptr(unsafe.Pointer(&psramEndSymbol))
	first := (start & mmuLinearMask) / mmuPageSize
	n := uintptr(0)
	for start+(n+1)*mmuPageSize <= end {
		e := volatile.LoadUint32((*uint32)(unsafe.Pointer(uintptr(mmuTable) + 4*(first+n))))
		if e&(mmuInvalid|mmuSpiram) != mmuSpiram || uintptr(e&mmuPhysMask) != n {
			break
		}
		n++
	}
	return start + n*mmuPageSize
}

// SpanStats returns the counters of each span: internal DRAM, and PSRAM (zero
// if PSRAM did not come up). ReadStats sums them.
func SpanStats() (dram, psram collector.Stats) {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return shim.spans.dram.Stats(), shim.spans.psram.Stats()
}
