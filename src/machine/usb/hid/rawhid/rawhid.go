// Package rawhid is a raw HID interface beside the keyboard: a vendor usage page with a 32-byte report each
// way, as QMK's, which is what Vial and VIA talk to a keyboard over.
package rawhid

import (
	"machine"
	"machine/usb"
	"machine/usb/descriptor"
	"machine/usb/hid"
	"runtime/volatile"
)

// ReportSize is the bytes in a report, each way.
const ReportSize = descriptor.RawHIDReportSize

var (
	in      [ReportSize]byte // the last report the host sent
	waiting volatile.Register8
	out     [ReportSize]byte
)

func init() {
	hid.Descriptor = descriptor.CDCHIDRaw
	machine.ConfigureUSBEndpoint(hid.Descriptor,
		[]usb.EndpointConfig{
			{
				Index:          usb.RAWHID_ENDPOINT_OUT,
				IsIn:           false,
				Type:           usb.ENDPOINT_TYPE_INTERRUPT,
				DelayRxHandler: received,
			},
			{
				Index: usb.RAWHID_ENDPOINT_IN,
				IsIn:  true,
				Type:  usb.ENDPOINT_TYPE_INTERRUPT,
			},
		},
		[]usb.SetupConfig{
			{
				Index:   usb.RAWHID_INTERFACE,
				Handler: hid.DefaultSetupHandler,
			},
		})
}

// received keeps a report from the host. The endpoint isn't made ready for another until it's been
// read, so the host waits, and none is lost or answered out of turn.
func received(b []byte) bool {
	n := copy(in[:], b)
	clear(in[n:])
	waiting.Set(1)
	return false
}

// Receive copies the report the host last sent into p, if there's one that hasn't been read.
func Receive(p *[ReportSize]byte) bool {
	if waiting.Get() == 0 {
		return false
	}
	*p = in
	waiting.Set(0)
	machine.AckUsbOutTransfer(usb.RAWHID_ENDPOINT_OUT)
	return true
}

// Send sends a report to the host.
func Send(p *[ReportSize]byte) {
	out = *p
	machine.SendUSBInPacket(usb.RAWHID_ENDPOINT_IN, out[:])
}
