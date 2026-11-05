//go:build esp32 || esp32c3 || esp32s3

package machine

// ledcApbClock is what LEDC counts: APB_CLK on these chips.
const ledcApbClock = 80_000000
