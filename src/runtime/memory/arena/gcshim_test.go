//go:build gc.custom

package arena

// gcshim supplies the collector of a -gc=custom build.
import _ "runtime/memory/gcshim"
