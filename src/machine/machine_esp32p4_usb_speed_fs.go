//go:build esp32p4 && usbotg && !usbhs

package machine

// Full speed on the high speed PHY. See DCFG.DevSpd in soc/esp32p4/include/soc/usb_dwc_struct.h.
const (
	dcfgDevSpeed = 1
	otgHighSpeed = false
)
