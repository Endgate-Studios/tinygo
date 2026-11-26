//go:build esp32s3

package radio

import (
	"device/esp"
	"time"
	"unsafe"
)

var boot = time.Now()

//export tinygo_wifi_esp_timer_get_time
func osiTimerGetTime() int64 {
	return time.Since(boot).Microseconds()
}

//export tinygo_wifi_log_timestamp
func osiLogTimestamp() uint32 {
	return uint32(time.Since(boot).Milliseconds())
}

// random reads the hardware random number generator. The RF subsystem
// makes it a true random source when Wi-Fi runs, see the ESP32-S3 Technical
// Reference Manual, chapter Random Number Generator.
func random() uint32 { return esp.RNG.DATA.Get() }

//export tinygo_wifi_rand
func osiRand() uint32 {
	return random()
}

//export tinygo_wifi_random
func osiRandom() uint32 {
	return random()
}

//export tinygo_wifi_get_random
func osiGetRandom(buf *byte, n uintptr) int32 {
	b := unsafe.Slice(buf, n)
	for i := 0; i < len(b); i += 4 {
		r := random()
		for j := i; j < i+4 && j < len(b); j++ {
			b[j] = byte(r)
			r >>= 8
		}
	}
	return 0
}

//export tinygo_wifi_nop
func osiNop() {
}

//export tinygo_wifi_log
func osiLog(text *byte, n int32) {
	if n > 0 {
		print(unsafe.String(text, n))
	}
}

// hexstr2bin is the wpa_supplicant helper that the blobs use. It returns -1
// if hex has a character that is not a hex digit.
//
//export hexstr2bin
func hexstr2bin(hex *byte, buf *byte, n uintptr) int32 {
	src := unsafe.Slice(hex, 2*n)
	dst := unsafe.Slice(buf, n)
	for i := range dst {
		hi, lo := hexDigit(src[2*i]), hexDigit(src[2*i+1])
		if hi < 0 || lo < 0 {
			return -1
		}
		dst[i] = byte(hi<<4 | lo)
	}
	return 0
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
