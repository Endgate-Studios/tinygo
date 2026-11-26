//go:build gc.custom && gcshim_psram

// Command psramgc checks the GC heap of gcshim over internal RAM and PSRAM. It keeps 3 MB of live
// blocks, more than internal RAM holds, checks them, drops half, collects and allocates again.
//
//	tinygo flash -gc=custom -tags=gcshim_psram,gcshim_precise_globals -target=esp32s3-psram-octal-jtag -port COM4 ./src/examples/psramgc
package main

import (
	"runtime"
	"time"

	_ "runtime/memory/gcshim" // installs the collector
)

const (
	blockSize = 64 << 10
	nBlocks   = 48 // 3 MB
)

var (
	blocks [nBlocks][]byte // the only references, so the GC must keep these blocks
	failed bool
)

func check(ok bool, name string) {
	if ok {
		println("PASS", name)
		return
	}
	println("FAIL", name)
	failed = true
}

func fill(i int) {
	b := make([]byte, blockSize)
	for j := range b {
		b[j] = byte(i + j)
	}
	blocks[i] = b
}

// intact reports whether every kept block still holds its pattern.
func intact() bool {
	for i, b := range blocks {
		if b == nil {
			continue
		}
		for j := range b {
			if b[j] != byte(i+j) {
				return false
			}
		}
	}
	return true
}

func stats(label string) runtime.MemStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	println(label, "HeapSys", int(ms.HeapSys>>10), "KiB, HeapInuse", int(ms.HeapInuse>>10), "KiB, NumGC", ms.NumGC)
	return ms
}

func main() {
	time.Sleep(3 * time.Second) // lets a serial reader attach after the USB port reconnects
	start := stats("start")
	check(start.HeapSys > 1<<20, "the GC heap includes PSRAM (more than 1 MiB)")

	for i := range blocks {
		fill(i)
	}
	full := stats("3 MB live")
	check(intact(), "48 blocks of 64 KiB allocated and intact")
	check(full.HeapInuse >= nBlocks*blockSize, "HeapInuse counts the 3 MB")

	for i := 0; i < nBlocks; i += 2 {
		blocks[i] = nil
	}
	t0 := time.Now()
	runtime.GC()
	pause := time.Since(t0)
	half := stats("after dropping half")
	println("  collection took", int(pause.Milliseconds()), "ms")
	check(intact(), "the kept half survives a collection")
	check(half.HeapInuse < full.HeapInuse-nBlocks/4*blockSize, "the dropped half is freed")

	for i := 0; i < nBlocks; i += 2 {
		fill(i)
	}
	stats("refilled")
	check(intact(), "the freed memory is used again, and all blocks are intact")

	if failed {
		println("SOME FAILED")
	} else {
		println("ALL PASS")
	}
}
