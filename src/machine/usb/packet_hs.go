//go:build usbhs

package usb

// EndpointPacketSize is a high speed bulk endpoint's, which USB 2.0 section 5.8.3 fixes at 512.
const EndpointPacketSize = 512
