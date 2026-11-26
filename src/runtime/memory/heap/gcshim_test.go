//go:build gc.custom

package heap

// gcshim supplies the collector of a -gc=custom build.
import _ "runtime/memory/gcshim"
