//go:build baremetal

package collector

import (
	"flag"
	"testing"
	"time"
)

// The pause lets a serial reader attach after the USB port reconnects, before the tests print.
// The board has no command line, so verbose output is set here, one line per test.
func init() {
	time.Sleep(3 * time.Second)
	testing.Init()
	flag.Set("test.v", "true")
}
