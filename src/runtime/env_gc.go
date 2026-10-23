//go:build !gc.none

package runtime

// makeEnvs returns a slice for n environment variables.
func makeEnvs(n int) []string {
	return make([]string, n)
}
