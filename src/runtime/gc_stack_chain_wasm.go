//go:build (gc.conservative || gc.custom || gc.precise || gc.boehm) && tinygo.wasm

package runtime

import "unsafe"

type stackChainObject struct {
	parent   *stackChainObject
	numSlots uintptr
}

// trackPointer is a stub function call inserted by the compiler during IR
// construction. Calls to it are later replaced with regular stack bookkeeping
// code.
func trackPointer(ptr, alloca unsafe.Pointer)

// swapStackChain swaps the stack chain.
// This is called from internal/task when switching goroutines.
func swapStackChain(dst **stackChainObject) {
	*dst, stackChainStart = stackChainStart, *dst
}
