//go:build !usbhs

package usb

// EndpointPacketSize is a full speed endpoint's, as USB 2.0 section 5.8.3 has for bulk.
const EndpointPacketSize = 64
