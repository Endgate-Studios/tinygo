package hci

import "testing"

// FuzzParse gives packets to all parse functions. None may panic.
func FuzzParse(f *testing.F) {
	f.Add([]byte{TypeEvent, EvtCommandComplete, 4, 1, 0x03, 0x0c, 0})
	f.Add([]byte{TypeACL, 0x40, 0x20, 5, 0, 1, 0, 4, 0, 0x0a})
	f.Add([]byte{TypeEvent, 0x3e, 12, 0x02, 1, 0, 0, 1, 2, 3, 4, 5, 6, 0, 0xc0})
	f.Fuzz(func(t *testing.T, b []byte) {
		ParseACL(b)
		ParseCommand(b)
		if e, err := ParseEvent(b); err == nil {
			ParseConnection(e.Params)
			ParseAdvReports(e.Params)
			ParseLTKRequest(e.Params)
			ParseConnectionUpdate(e.Params)
		}
		if len(b) > 0 {
			ParseConnection(b[1:])
			ParseAdvReports(b[1:])
		}
	})
}
