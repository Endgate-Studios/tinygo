//go:build !numa_psram_octal

package main

func psramBuf() []byte { return nil }
