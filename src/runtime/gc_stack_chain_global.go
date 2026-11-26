//go:build (gc.conservative || gc.custom || gc.precise || gc.boehm) && tinygo.wasm && !tinygo.wasm.cores

package runtime

//go:extern runtime.stackChainStart
var stackChainStart *stackChainObject
