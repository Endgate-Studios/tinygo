//go:build gc.custom

package free

// gcshim supplies the collector of a -gc=custom build.
import _ "runtime/memory/gcshim"
