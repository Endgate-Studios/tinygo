//go:build !numa_psram_octal

package main

func psramArena() []byte { return nil }
