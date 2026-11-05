//go:build esp32p4 && !(esp32p4 && usbotg)

package machine

func usbLeaveBus() {}
