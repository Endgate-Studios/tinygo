//go:build gc.custom && tinygo.wasm && !gcshim_heap16m && !gcshim_heap512m

package gcshim

// maxHeapSize is the wasm heap limit. Metadata for it is committed at start-up.
// The gcshim_heap16m and gcshim_heap512m tags select a different limit.
const maxHeapSize = 64 << 20
