package descriptor

// Raw HID is a vendor usage page with a 32-byte report each way and no report ID, as QMK's: usage page
// 0xFF60, usage 0x61, its input 0x62 and its output 0x63. Vial and VIA look for a keyboard by these.
var rawHIDReport = []byte{
	0x06, 0x60, 0xFF, // usage page 0xFF60
	0x09, 0x61, //       usage 0x61
	0xA1, 0x01, //       collection: application

	0x09, 0x62, //       usage 0x62, what the device sends
	0x15, 0x00, //       logical minimum 0
	0x26, 0xFF, 0x00, // logical maximum 255
	0x95, RawHIDReportSize,
	0x75, 0x08, //       8 bits each
	0x81, 0x02, //       input: data, variable, absolute

	0x09, 0x63, //       usage 0x63, what the host sends
	0x15, 0x00,
	0x26, 0xFF, 0x00,
	0x95, RawHIDReportSize,
	0x75, 0x08,
	0x91, 0x02, //       output: data, variable, absolute

	0xC0, // end collection
}

// RawHIDReportSize is the bytes in a raw HID report, each way.
const RawHIDReportSize = 32

var configurationCDCHIDRaw = [configurationTypeLen]byte{
	configurationTypeLen,
	TypeConfiguration,
	0x00, 0x00, // set by Configure
	0x04, // number of interfaces
	0x01, // configuration value
	0x00, // index to string description
	0xa0, // attributes
	0x32, // maxpower
}

var interfaceRawHID = [interfaceTypeLen]byte{
	interfaceTypeLen,
	TypeInterface,
	0x03, // InterfaceNumber
	0x00, // AlternateSetting
	0x02, // NumEndpoints
	0x03, // InterfaceClass: HID
	0x00, // InterfaceSubClass: not a boot device
	0x00, // InterfaceProtocol
	0x00, // Interface
}

var classRawHID = [ClassHIDTypeLen]byte{
	ClassHIDTypeLen,
	TypeClassHID,
	0x11, // HID version L
	0x01, // HID version H
	0x00, // CountryCode
	0x01, // NumDescriptors
	0x22, // ClassType
	byte(len(rawHIDReport)),
	0x00,
}

// CDCHIDRaw is CDCHID with a raw HID interface after it:
//
// EP1 IN : CDC Call Management
// EP2 OUT: CDC OUT
// EP2 IN : CDC IN
// EP3 OUT: HID OUT
// EP3 IN : HID IN
// EP4 OUT: raw HID OUT
// EP4 IN : raw HID IN
var CDCHIDRaw = Descriptor{
	Device: DeviceCDC.Bytes(),
	Configuration: Append([][]byte{
		configurationCDCHIDRaw[:],
		CDCHID.Configuration[configurationTypeLen:],
		interfaceRawHID[:],
		classRawHID[:],
		EndpointIN(EndpointEP4, TransferTypeInterrupt, 0x40, 0x01).Bytes(),
		EndpointOUT(EndpointEP4, TransferTypeInterrupt, 0x40, 0x01).Bytes(),
	}),
	HID: map[uint16][]byte{
		2: CDCHID.HID[2],
		3: rawHIDReport,
	},
}
