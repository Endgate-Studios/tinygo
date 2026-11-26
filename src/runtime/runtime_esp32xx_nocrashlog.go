//go:build (esp32 || esp32c3 || esp32c6 || esp32h2 || esp32p4) && !(esp32p4 && usbotg && serial.usb)

package runtime

func crashKeep(c byte) {}

func crashRestart() {}
