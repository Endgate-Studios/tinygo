//go:build gc.custom

package main

// -gc=custom: gcshim's collector, over its default 64MB heap.
import _ "runtime/memory/gcshim"
