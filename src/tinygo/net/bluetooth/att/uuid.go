package att

// UUID is a Bluetooth UUID in wire order, least significant byte first.
type UUID [16]byte

// base is the Bluetooth Base UUID 00000000-0000-1000-8000-00805F9B34FB in wire order
// (Core spec v5.4 Vol 3 Part B section 2.5.1).
var base = UUID{0xfb, 0x34, 0x9b, 0x5f, 0x80, 0x00, 0x00, 0x80, 0x00, 0x10, 0x00, 0x00, 0, 0, 0, 0}

// UUID16 returns the full UUID of a 16-bit assigned number.
func UUID16(v uint16) UUID {
	u := base
	u[12], u[13] = byte(v), byte(v>>8)
	return u
}

// Short returns the 16-bit form, and ok is false when u is not based on the Bluetooth Base UUID.
func (u UUID) Short() (uint16, bool) {
	if u[14] != 0 || u[15] != 0 {
		return 0, false
	}
	for i := 0; i < 12; i++ {
		if u[i] != base[i] {
			return 0, false
		}
	}
	return uint16(u[12]) | uint16(u[13])<<8, true
}

// Len returns the wire length of u: 2 for a 16-bit UUID, 16 otherwise.
func (u UUID) Len() int {
	if _, ok := u.Short(); ok {
		return 2
	}
	return 16
}

// put writes u in its short wire form into dst and returns the length.
func (u *UUID) put(dst []byte) int {
	if s, ok := u.Short(); ok {
		dst[0], dst[1] = byte(s), byte(s>>8)
		return 2
	}
	return copy(dst, u[:])
}

// parseUUID reads a 2 or 16-byte wire UUID.
func parseUUID(b []byte) (UUID, bool) {
	switch len(b) {
	case 2:
		return UUID16(uint16(b[0]) | uint16(b[1])<<8), true
	case 16:
		var u UUID
		copy(u[:], b)
		return u, true
	}
	return UUID{}, false
}

// GATT attribute types (Assigned Numbers section 3.5).
var (
	TypePrimaryService = UUID16(0x2800)
	TypeCharacteristic = UUID16(0x2803)
	TypeCCCD           = UUID16(0x2902)
)
