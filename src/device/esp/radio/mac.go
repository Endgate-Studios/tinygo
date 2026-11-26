//go:build esp32s3

package radio

import (
	"device/esp"
	"unsafe"
)

// MAC returns the factory MAC address of the chip, as ESP-IDF
// esp_efuse_mac_get_default. It is in eFuse BLOCK1, see the ESP32-S3
// Technical Reference Manual, chapter eFuse Controller.
func MAC() [6]byte {
	lo, hi := esp.EFUSE.RD_MAC_SPI_SYS_0.Get(), esp.EFUSE.RD_MAC_SPI_SYS_1.Get()
	return [6]byte{byte(hi >> 8), byte(hi), byte(lo >> 24), byte(lo >> 16), byte(lo >> 8), byte(lo)}
}

// _read_mac gives the MAC of an interface: the Wi-Fi station (0), the soft
// access point (1), Bluetooth (2) or Ethernet (3). The chip has 4 universal
// addresses, so the type is added to the last byte, as ESP-IDF esp_read_mac.
//
//export tinygo_wifi_read_mac
func osiReadMAC(mac *byte, typ uint32) int32 {
	if typ > 3 {
		return -1
	}
	m := MAC()
	m[5] += byte(typ)
	copy(unsafe.Slice(mac, 6), m[:])
	return 0
}
