//go:build bcm2710 && !scheduler.cores

package runtime

// bcm2710CrossCore is the mailbox 0 handler. One core sends nothing to itself.
func bcm2710CrossCore(core uint32) {}

// runSecondary is never reached: only the cores scheduler starts cores 1 to 3.
//
//export tinygo_runSecondary
func runSecondary(core uint32) {
	abort()
}
