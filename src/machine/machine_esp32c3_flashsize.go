//go:build esp32c3

package machine

// flashChipSize is 0: this chip goes by its image header.
func flashChipSize() uint32 { return 0 }

func flashEnd() uintptr { return FlashDataEnd() }
