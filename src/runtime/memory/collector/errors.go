package collector

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	// ErrBadPageSize is the Init error when Config.PageSize is not a power
	// of two of at least MinPageSize.
	ErrBadPageSize Error = "collector: page size must be a power of two >= 256"

	// ErrHeapTooSmall is the Init error when the span cannot hold the
	// metadata and one page.
	ErrHeapTooSmall Error = "collector: heap too small for metadata and one page"

	// ErrCorrupt is the Check error when an invariant is not true.
	ErrCorrupt Error = "collector: heap invariant violated"

	// ErrBadSet is the NewSet error when the primary is nil, or when the spans are the same or overlap.
	ErrBadSet Error = "collector: set needs a primary and spans that do not overlap"
)
