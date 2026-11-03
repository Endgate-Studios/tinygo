package collector

import "unsafe"

// Check validates the heap invariants and returns ErrCorrupt if one fails.
// It is slow. Use it in tests and debug builds.
func (c *Collector) Check() error {
	free := uintptr(0)
	for page := uintptr(0); page < c.nPages; {
		switch k := c.kinds[page]; {
		case k == kindFree:
			free++
			page++
		case k >= kindSmall:
			if int(k-kindSmall) >= numClasses || c.slots[k-kindSmall] == 0 {
				return ErrCorrupt
			}
			page++
		case k == kindLargeHead || k == kindManualHead || k == kindBytesHead:
			n := uintptr(c.runs[page])
			if n == 0 || page+n > c.nPages {
				return ErrCorrupt
			}
			tail := kindLargeTail
			if k == kindBytesHead {
				tail = kindBytesTail
			}
			if k == kindManualHead {
				tail = kindManualTail
			}
			for q := page + 1; q < page+n; q++ {
				if c.kinds[q] != tail || uintptr(c.runs[q]) != page {
					return ErrCorrupt
				}
			}
			page += n
		default:
			return ErrCorrupt // orphan tail page
		}
	}
	if free != c.freePages {
		return ErrCorrupt
	}

	// The mark bits are the visited set for the free list walk. This is safe
	// because Check never runs during a cycle.
	clear(c.usedMarkBits())
	for cls := 0; cls < numClasses; cls++ {
		for slot := c.freeLists[cls]; slot != 0; slot = *(*uintptr)(unsafe.Pointer(slot)) {
			obj, ok := c.objectAt(slot)
			if !ok || obj != slot {
				return ErrCorrupt
			}
			page := (slot - c.pagesStart) / c.cfg.PageSize
			g := c.granuleOf(slot)
			if c.kinds[page] != kindSmall+uint8(cls) || getBit(c.allocBits, g) || getBit(c.markBits, g) {
				return ErrCorrupt
			}
			setBit(c.markBits, g)
		}
	}
	clear(c.usedMarkBits())
	return nil
}
