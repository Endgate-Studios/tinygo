//go:build gc.custom

package memory

// gcshim supplies the collector of a -gc=custom build.
import _ "runtime/memory/gcshim"
