//go:build usbhs

package cdc

// ringBufLen is a power of 2, with room for several high speed packets.
const ringBufLen = 8192
