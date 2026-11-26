//go:build esp32s3 && !scheduler.cores

package nn

func enableDualCore() {}

func dualReady() bool { return false }

func dualRun(n int, f func(scratch []byte)) bool { return false }

func dualWait() {}
