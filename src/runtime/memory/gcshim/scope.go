//go:build gc.custom && (tinygo.wasm || (baremetal && (esp32 || esp32s3 || esp32c3 || esp32p4 || scheduler.cores)) || (windows && amd64 && scheduler.tasks))

package gcshim

import (
	"internal/task"
	"unsafe"
)

// Scope is memory that takes a goroutine's allocations while the goroutine
// is inside it: for data that dies all at once, a UI's widgets, say, made
// anew on every rebuild. Reset throws it all away at once, with no
// collection, and allocating there brings none on: a collection stops every
// core.
//
// What a Scope holds is scanned as a root, so the heap objects it points at
// stay alive. Nothing may point into it after Reset: data made inside that
// must last has to be made outside it (Exit, allocate, Enter). Reset fills
// it with poison, so a pointer kept by mistake fails at once rather than
// reading another's data. Allocations that don't fit go to the heap.
type Scope struct {
	base, top, end uintptr
	owner          *task.Task // inside it, or nil
	cpu            uint32     // the core owner was on when it entered

	// Overflows counts the allocations that didn't fit and went to the
	// heap; Peak is the most it has held.
	Overflows uint32
	Peak      uintptr
}

// scopePoison fills a Scope at Reset: as a pointer it is no address, so a
// kept pointer read after Reset faults.
const scopePoison = 0xA5

// Each Scope is a Root, so Scopes count against MaxRoots.
var scopes struct {
	inside [2]*Scope // by core: the Scope being allocated into
}

// NewScope makes a Scope of mem, which must not be the heap's (a fixed
// array, best in PSRAM). It returns nil when MaxRoots Roots are registered.
func NewScope(mem []byte) *Scope {
	if len(mem) < 16 {
		return nil
	}
	lo := uintptr(unsafe.Pointer(&mem[0]))
	base := (lo + 7) &^ 7
	// Made before taking the lock: allocating takes it too.
	s := &Scope{base: base, top: base, end: (lo + uintptr(len(mem))) &^ 7}
	shim.lock.Lock()
	defer shim.lock.Unlock()
	if addRootLocked(s) != nil {
		return nil
	}
	return s
}

// Live is the part of s in use. It is for the collector (see Root): use
// Used, which takes the lock.
func (s *Scope) Live() (start, end uintptr) { return s.base, s.top }

// Enter has this goroutine's allocations go to s, until Exit. Only one
// goroutine on a core can be inside a Scope at a time; another goroutine
// on the core allocates from the heap as usual.
func (s *Scope) Enter() {
	shim.lock.Lock()
	s.owner, s.cpu = task.Current(), thisCPU()
	scopes.inside[s.cpu] = s
	shim.lock.Unlock()
}

// Exit has this goroutine's allocations go to the heap again.
func (s *Scope) Exit() {
	shim.lock.Lock()
	if scopes.inside[s.cpu] == s {
		scopes.inside[s.cpu] = nil
	}
	s.owner = nil
	shim.lock.Unlock()
}

// Reset throws away all s holds. Nothing may point into it any more.
func (s *Scope) Reset() {
	shim.lock.Lock()
	n := s.top - s.base
	shim.lock.Unlock()
	memset(unsafe.Pointer(s.base), scopePoison, n)
	shim.lock.Lock()
	s.top = s.base
	shim.lock.Unlock()
}

// Used is how much s holds.
func (s *Scope) Used() uintptr {
	shim.lock.Lock()
	defer shim.lock.Unlock()
	return s.top - s.base
}

// scopeAlloc allocates size zeroed bytes in the Scope this goroutine is
// inside, if any and if they fit; nil otherwise. The caller holds shim.lock.
func scopeAlloc(size uintptr) unsafe.Pointer {
	s := scopes.inside[thisCPU()]
	if s == nil || s.owner != task.Current() {
		return nil
	}
	size = (size + 7) &^ 7
	if size > s.end-s.top {
		s.Overflows++
		return nil
	}
	p := unsafe.Pointer(s.top)
	s.top += size
	s.Peak = max(s.Peak, s.top-s.base)
	memset(p, 0, size)
	return p
}

func memset(p unsafe.Pointer, b byte, n uintptr) {
	s := unsafe.Slice((*byte)(p), n)
	if b == 0 {
		clear(s)
		return
	}
	for i := range s {
		s[i] = b
	}
}
