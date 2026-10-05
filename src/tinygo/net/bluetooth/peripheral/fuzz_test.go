package peripheral

import (
	"encoding/binary"
	"testing"

	"tinygo/net/bluetooth/hci"
)

// lenient moves the packets of the peripheral like pump, but it accepts any
// packet: each command gets a Command Complete, and ACL data is dropped.
func (h *harness) lenient() {
	for round := 0; len(h.out) > 0 && round < 64; round++ {
		out := h.out
		h.out = nil
		for _, b := range out {
			if len(b) >= 3 && b[0] == hci.TypeCommand {
				p := [4]byte{1, b[1], b[2], 0}
				var e [16]byte
				n, _ := hci.PutEvent(e[:], hci.EvtCommandComplete, p[:])
				h.p.HandleHCI(e[:n], h.now)
			}
		}
	}
}

// FuzzPeripheral connects a central, then gives the peripheral a series of
// packets from it. Each step is a kind, a channel byte, a length and data:
// 0 is one L2CAP frame on the ATT, signaling, SMP or another channel, 1 a
// continuation fragment, 2 a raw HCI packet, 3 a step of the clock.
func FuzzPeripheral(f *testing.F) {
	f.Add([]byte{0, 0, 3, 0x02, 0x40, 0x00})                      // ATT Exchange MTU
	f.Add([]byte{0, 0, 7, 0x08, 1, 0, 0xff, 0xff, 0x03, 0x28})    // Read By Type
	f.Add([]byte{0, 2, 7, 0x01, 0x03, 0, 0x01, 0x10, 0x07, 0x07}) // Pairing Request
	f.Add([]byte{0, 1, 6, 0x12, 1, 2, 0, 0x10, 0})                // Connection Parameter Update
	f.Add([]byte{3, 0, 0, 2, 0, 1, 0x05, 0x04, 0x00, 0x40, 0x00, 0x13})
	f.Fuzz(func(t *testing.T, steps []byte) {
		if len(steps) > 2048 {
			return
		}
		h := newHarness(t, &store{})
		h.start()
		h.connect(centralID)
		h.lenient()
		var pkt [300]byte
		for len(steps) >= 3 {
			kind, ch, n := steps[0]&3, steps[1], int(steps[2])
			steps = steps[3:]
			n = min(n, len(steps), 251)
			data := steps[:n]
			steps = steps[n:]
			var b []byte
			switch kind {
			case 0:
				cid := []uint16{0x0004, 0x0005, 0x0006, uint16(ch) << 4}[ch&3]
				b = append(pkt[:0], hci.TypeACL, 0, 0, 0, 0, 0, 0, 0, 0)
				binary.LittleEndian.PutUint16(b[1:], conn|0x2000)
				binary.LittleEndian.PutUint16(b[3:], uint16(4+n))
				binary.LittleEndian.PutUint16(b[5:], uint16(n))
				binary.LittleEndian.PutUint16(b[7:], cid)
				b = append(b, data...)
			case 1:
				b = append(pkt[:0], hci.TypeACL, 0, 0, 0, 0)
				binary.LittleEndian.PutUint16(b[1:], conn|0x1000)
				binary.LittleEndian.PutUint16(b[3:], uint16(n))
				b = append(b, data...)
			case 2:
				b = data
			case 3:
				h.now += uint32(ch) * 100
				continue
			}
			h.p.HandleHCI(b, h.now)
			h.lenient()
		}
		h.p.Close()
	})
}
