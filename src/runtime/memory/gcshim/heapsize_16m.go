//go:build gc.custom && tinygo.wasm && gcshim_heap16m

package gcshim

const maxHeapSize = 16 << 20
