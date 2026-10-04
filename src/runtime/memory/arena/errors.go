package arena

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrArenaTooSmall Error = "arena: memory too small"
	ErrEmptyRegion   Error = "arena: region memory is empty"
	ErrNilLocker     Error = "arena: locker cannot be nil"
	ErrTooManySpans  Error = "arena: heap has MaxSpans spans"

	errBlockSize Error = "arena: block has a bad size"
	errBlockTags Error = "arena: block header and footer differ"
	errAdjacent  Error = "arena: two free blocks are adjacent"
	errEpilogue  Error = "arena: block walk does not end at the epilogue"
	errFreeList  Error = "arena: free list is corrupt"
	errCounters  Error = "arena: counters do not match the block walk"

	errInvalidFree Error = "arena: free of pointer not allocated by this heap"
	errDoubleFree  Error = "arena: double free"
)
