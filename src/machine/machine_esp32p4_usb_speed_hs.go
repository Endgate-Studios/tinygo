//go:build esp32p4 && usbotg && usbhs

package machine

// High speed. See DCFG.DevSpd in soc/esp32p4/include/soc/usb_dwc_struct.h.
const (
	dcfgDevSpeed = 0
	otgHighSpeed = true
)
