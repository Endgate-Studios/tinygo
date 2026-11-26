//go:build baremetal

package main

import "time"

// A baremetal heap holds few goroutine stacks, 8 KiB each on the ESP32-S3 (targets/esp32s3.json).
// The pause lets a serial reader attach after the USB port reconnects.
func init() {
	scale = 100
	batch = 10
	time.Sleep(3 * time.Second)
}
