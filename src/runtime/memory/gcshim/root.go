package gcshim

// Root is memory the collector cannot see into by type. An arena.Region over
// a byte array is one: the collector sees bytes, not the objects the program
// put there with unsafe. With precise globals, it never reads such an array,
// in internal RAM or in PSRAM.
//
// AddRoot has the collector scan the live part of a Root conservatively on
// every collection, so the heap objects that the Root points at stay alive.
// A Scope is a Root too.
type Root interface {
	// Live is the part of the memory in use, [start, end). It runs during
	// a collection, with the world stopped and the heap lock held: it must
	// not allocate, lock or block.
	Live() (start, end uintptr)
}

// MaxRoots is how many Roots can be registered at once, Scopes included.
const MaxRoots = 8

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrTooManyRoots Error = "gcshim: MaxRoots roots are registered"
	ErrNotInstalled Error = "gcshim: gcshim is not the collector (build with -gc=custom)"
)
