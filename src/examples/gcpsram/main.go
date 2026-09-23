package main

// Does the collector see a pointer stored in PSRAM?
//
// The maintainer's first correctness item says it does not: ".psram isn't in the GC root
// set (_globals_start/_globals_end = _sbss/_edata), so a //go:section .psram variable
// holding a pointer or slice lets the GC free what it points at."
//
// Current dev marks globals from compiler-generated pointer layouts rather than from that
// address range - gcGlobalRoot in gc_globals_blocks.go - and _globals_start has no
// consumers left. So the merge may already have settled it. This is the difference between
// believing that and knowing it.
//
// The test: put the only reference to a heap object in a PSRAM variable, drop every other
// reference, collect repeatedly while allocating enough to reuse any freed block, then
// check the object still holds what was written. If the collector cannot see the pointer,
// the object is freed and its memory handed to one of the later allocations, which
// overwrite it.

import (
	"runtime"
	"time"
	"unsafe"
)

type payload struct {
	magic  uint32
	filler [64]byte
}

const magic = 0xC0FFEE01

// The only reference to the object lives here, in PSRAM.
//
//go:section .psram
var reference *payload

//go:section .psram
var slabInPSRAM [256 * 1024]byte

func main() {
	time.Sleep(2 * time.Second)
	println("=== GC root test ===")
	println("psram var address:", uint32(uintptr(unsafe.Pointer(&reference))))
	println("psram slab address:", uint32(uintptr(unsafe.Pointer(&slabInPSRAM[0]))))

	// Allocate on the heap and keep the only pointer in PSRAM.
	object := &payload{magic: magic}
	reference = object
	object = nil

	println("heap object address:", uint32(uintptr(unsafe.Pointer(reference))))

	// Collect, then churn the heap hard enough that a freed block is certain to be reused.
	for round := 0; round < 8; round++ {
		runtime.GC()
		for i := 0; i < 400; i++ {
			junk := make([]byte, 96)
			for j := range junk {
				junk[j] = 0x5A
			}
			_ = junk
		}
	}
	runtime.GC()

	if reference.magic == magic {
		println("SURVIVED: the collector sees pointers in .psram")
	} else {
		println("CORRUPTED: magic is now", reference.magic, "- .psram is not a GC root")
	}

	// And the plain-data case, which is what the wake word actually needs.
	for i := range slabInPSRAM {
		slabInPSRAM[i] = byte(i)
	}
	bad := 0
	for i := range slabInPSRAM {
		if slabInPSRAM[i] != byte(i) {
			bad++
		}
	}
	println("plain psram slab bad bytes:", bad)
	println("=== done ===")
}
