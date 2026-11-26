//go:build (gc.conservative || gc.custom || gc.precise || gc.boehm) && tinygo.wasm && tinygo.wasm.cores

package runtime

// Each core walks its own stack chain.
//
//go:extern runtime.stackChainStart
//go:threadlocal
var stackChainStart *stackChainObject
