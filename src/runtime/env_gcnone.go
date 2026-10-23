//go:build gc.none

package runtime

// maxEnvs is the number of environment variables that syscall gets with
// -gc=none. It does not get the variables after them.
const maxEnvs = 256

var envsStatic [maxEnvs]string

// makeEnvs gives a static slice, so that package time can read TZ without an
// allocation.
func makeEnvs(n int) []string {
	return envsStatic[:min(n, maxEnvs)]
}
