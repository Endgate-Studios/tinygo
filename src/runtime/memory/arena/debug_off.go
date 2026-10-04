//go:build !memory_debug

package arena

// debugPoison is the default for WithPoison. The memory_debug tag sets it to true.
const debugPoison = false
