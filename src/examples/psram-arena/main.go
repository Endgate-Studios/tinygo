//go:build numa_psram_octal

// Command psram checks the PSRAM of an ESP32-S3 with a 4 MB pool in the .psram section: a pattern
// over all of it, its speed against internal RAM, and an arena.Heap with random allocations.
//
//	tinygo flash -target=esp32s3-psram-octal-jtag -port COM4 ./src/examples/psram-arena
package main

import (
	"math/rand"
	"time"
	"unsafe"

	"runtime/memory/arena"
)

// psramPool is in PSRAM. The .psram section comes from targets/esp32s3.ld.
//
//go:section .psram
var psramPool [4 << 20]byte

var dramBuf [64 << 10]byte // internal RAM, for the speed comparison

var failed bool

func check(ok bool, name string) {
	if ok {
		println("PASS", name)
		return
	}
	println("FAIL", name)
	failed = true
}

// words returns b as 32-bit words.
func words(b []byte) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Pointer(&b[0])), len(b)/4)
}

// pattern writes a value made from the address of each word, then reads all words back.
// A wrong mapping or a bad address line shows as a wrong value at some address.
func pattern(b []byte, salt uint32) (errors int) {
	w := words(b)
	base := uint32(uintptr(unsafe.Pointer(&b[0])))
	for i := range w {
		w[i] = (base + uint32(i)*4) ^ salt
	}
	for i, v := range w {
		if v != (base+uint32(i)*4)^salt {
			errors++
		}
	}
	return errors
}

// speed returns write and read throughput in KB/s over total bytes, in passes over b.
func speed(b []byte, total int) (writeKBs, readKBs int) {
	w := words(b)
	passes := total / len(b)
	t0 := time.Now()
	for p := 0; p < passes; p++ {
		for i := range w {
			w[i] = uint32(i + p)
		}
	}
	wt := time.Since(t0)
	var sum uint32
	t0 = time.Now()
	for p := 0; p < passes; p++ {
		for _, v := range w {
			sum += v
		}
	}
	rt := time.Since(t0)
	if sum == 1 {
		println() // keeps the reads
	}
	return int(int64(total) * 1000 / (wt.Microseconds() + 1)), int(int64(total) * 1000 / (rt.Microseconds() + 1))
}

type block struct {
	p    unsafe.Pointer
	size uintptr
	tag  byte
}

// arenaTest runs random allocations and frees in a 4 MB arena.Heap over PSRAM, and checks each block.
func arenaTest() {
	h, err := arena.NewHeap(psramPool[:])
	if err != nil {
		println("arena:", err.Error())
		failed = true
		return
	}
	rng := rand.New(rand.NewSource(1))
	var live [512]block
	n, corrupt, allocs := 0, 0, 0
	free := func(i int) {
		b := live[i]
		s := unsafe.Slice((*byte)(b.p), b.size)
		for _, c := range s {
			if c != b.tag {
				corrupt++
				break
			}
		}
		h.Free(b.p)
		live[i] = live[n-1]
		n--
	}
	for step := 0; step < 20000; step++ {
		if n > 0 && (n == len(live) || rng.Intn(3) == 0) {
			free(rng.Intn(n))
			continue
		}
		size := uintptr(1 + rng.Intn(16<<10))
		p := h.Alloc(size, 8)
		if p == nil {
			if n > 0 {
				free(rng.Intn(n))
			}
			continue
		}
		tag := byte(1 + rng.Intn(255))
		s := unsafe.Slice((*byte)(p), size)
		for i := range s {
			s[i] = tag
		}
		live[n] = block{p, size, tag}
		n++
		allocs++
	}
	peak := h.Stats().Peak
	for n > 0 {
		free(n - 1)
	}
	st := h.Stats()
	println("arena allocs", allocs, "peak", int(peak), "in use at end", int(st.InUse), "corrupt blocks", corrupt)
	check(corrupt == 0 && st.InUse == 0 && h.Check() == nil, "arena in PSRAM: 20000 random steps, all blocks intact and freed")
}

func main() {
	time.Sleep(3 * time.Second) // lets a serial reader attach after the USB port reconnects
	println("psram: pool at", uintptr(unsafe.Pointer(&psramPool[0])), "size", len(psramPool))

	errs := pattern(psramPool[:], 0) + pattern(psramPool[:], 0xa5a5a5a5)
	check(errs == 0, "pattern over 4 MB of PSRAM, two passes")
	if errs != 0 {
		println("  wrong words:", errs)
	}

	pw, pr := speed(psramPool[:], 4<<20)
	dw, dr := speed(dramBuf[:], 4<<20)
	println("speed KB/s: PSRAM write", pw, "read", pr, "| internal RAM write", dw, "read", dr)

	arenaTest()

	if failed {
		println("SOME FAILED")
	} else {
		println("ALL PASS")
	}
}
