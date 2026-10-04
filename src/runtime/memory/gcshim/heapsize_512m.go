//go:build gc.custom && tinygo.wasm && gcshim_heap512m && !gcshim_heap16m

package gcshim

const maxHeapSize = 512 << 20
