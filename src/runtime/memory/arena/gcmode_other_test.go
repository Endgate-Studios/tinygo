//go:build !gc.conservative && !gc.precise

package arena

// The GC tests skip the other collectors. gcsuite has the gcshim cases.
const (
	gcKnown      = false
	gcReadsBytes = false
)
