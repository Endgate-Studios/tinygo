package heap

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilFreer     Error = "heap: freer cannot be nil"
	ErrNilCollector Error = "heap: collector cannot be nil"
)
