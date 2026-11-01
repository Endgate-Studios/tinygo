// Package memtest has test helpers for the memory packages. A victim is a GC heap object
// with a known pattern. When the collector frees a victim that is still in use, a new
// object takes its memory and changes the pattern.
package memtest

import (
	"runtime"
	"unsafe"
)

// Victim is a GC heap object with a known pattern.
type Victim struct {
	canary [8]uintptr
}

const (
	// Canary is the pattern of a live victim. ReuseFreed writes a different one.
	Canary = 0xA5A5A5A5
	reused = 0x5A5A5A5A

	// Victims is the number of victims a test usually plants.
	Victims = 16
)

var sink *Victim

// Plant puts a new victim in each slot.
//
//go:noinline
func Plant(slots []unsafe.Pointer) {
	for i := range slots {
		v := new(Victim)
		for j := range v.canary {
			v.canary[j] = Canary
		}
		slots[i] = unsafe.Pointer(v)
	}
}

// ClobberStack writes over old copies of the victim pointers on the stack.
//
//go:noinline
func ClobberStack() uintptr {
	var buf [256]uintptr
	for i := range buf {
		buf[i] = uintptr(i)
	}
	return buf[len(buf)-1]
}

// ReuseFreed allocates objects of the victim size with a different pattern.
//
//go:noinline
func ReuseFreed() {
	for range 2000 {
		v := new(Victim)
		for j := range v.canary {
			v.canary[j] = reused
		}
		sink = v
	}
	sink = nil
}

// Corrupted returns the number of victims in slots without the Canary pattern.
func Corrupted(slots []unsafe.Pointer) int {
	n := 0
	for _, p := range slots {
		for _, w := range (*Victim)(p).canary {
			if w != Canary {
				n++
				break
			}
		}
	}
	return n
}

// Collect runs up to rounds of runtime.GC and ReuseFreed, and stops at the first corrupted
// victim. The allocator can use freed blocks again only after some rounds, so 8 is usual.
// It returns the number of corrupted victims.
func Collect(slots []unsafe.Pointer, rounds int) int {
	ClobberStack()
	bad := 0
	for round := 0; round < rounds && bad == 0; round++ {
		runtime.GC()
		ReuseFreed()
		bad = Corrupted(slots)
	}
	return bad
}
