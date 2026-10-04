package peripheral

import (
	"math/rand"
	"testing"

	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

type cccdStore struct{ saved map[smp.Address][]uint16 }

func (c *cccdStore) LoadCCCDs(peer *smp.Address, dst []uint16) bool {
	v, ok := c.saved[*peer]
	copy(dst, v)
	return ok
}

func (c *cccdStore) SaveCCCDs(peer *smp.Address, values []uint16) error {
	c.saved[*peer] = append([]uint16(nil), values...)
	return nil
}

// bondAndSubscribe pairs with identity keys, subscribes to the battery level, and disconnects.
func bondAndSubscribe(t *testing.T, h *harness) {
	t.Helper()
	h.start()
	peer := smp.Address{Type: 1, Addr: [6]byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x76}}
	h.connect(peer)
	h.pair(peer)
	h.ltkRequest()
	h.encrypted()
	h.pop(l2cap.CIDSMP, 0x08)
	h.pop(l2cap.CIDSMP, 0x09)
	h.send(l2cap.CIDSMP, cat([]byte{0x08}, centralIRK[:]))
	h.send(l2cap.CIDSMP, cat([]byte{0x09, centralID.Type}, centralID.Addr[:]))
	h.send(l2cap.CIDATT, []byte{0x12, 0x04, 0x00, 0x01, 0x00}) // CCCD write, notifications on
	h.pop(l2cap.CIDATT, 0x13)
	h.event(hci.EvtDisconnectionComplete, []byte{0, byte(conn), byte(conn >> 8), 0x13})
	h.command(hci.OpLESetAdvEnable)
}

// reconnect connects from a new private address of the central and encrypts with the stored bond.
func reconnect(t *testing.T, h *harness) {
	t.Helper()
	var rpa [6]byte
	if err := smp.NewRPA(h.cmem, &centralIRK, rand.New(rand.NewSource(8)), &rpa); err != nil {
		t.Fatal(err)
	}
	h.connect(smp.Address{Type: 1, Addr: rpa})
	if c := h.ltkRequest(); c.Op != hci.OpLELongTermKeyReply {
		t.Fatalf("no LTK for the bonded peer, command %#04x", uint16(c.Op))
	}
	h.encrypted()
}

func TestCCCDKeptWithBond(t *testing.T) {
	h := newHarness(t, &store{})
	cs := &cccdStore{saved: map[smp.Address][]uint16{}}
	h.p.cfg.CCCDs = cs
	bondAndSubscribe(t, h)
	if v := cs.saved[centralID]; len(v) != 1 || v[0] != 1 {
		t.Fatalf("saved CCCDs %v under the identity address, want [1]", cs.saved)
	}
	reconnect(t, h)
	if err := h.p.Server().Notify(3, []byte{55}); err != nil {
		t.Fatalf("notify after reconnection: %v", err)
	}
	h.pump()
	if n := h.pop(l2cap.CIDATT, 0x1b); n[3] != 55 {
		t.Errorf("notification % x", n)
	}
	inUse := h.mem.Stats().InUse
	h.event(hci.EvtDisconnectionComplete, []byte{0, byte(conn), byte(conn >> 8), 0x13})
	if h.mem.Stats().InUse > inUse {
		t.Errorf("memory in use grew from %d to %d over a disconnection", inUse, h.mem.Stats().InUse)
	}
}

func TestCCCDWithoutStore(t *testing.T) {
	h := newHarness(t, &store{})
	bondAndSubscribe(t, h)
	reconnect(t, h)
	if err := h.p.Server().Notify(3, []byte{55}); err != att.ErrNotifyOff {
		t.Errorf("notify with no store: %v, want ErrNotifyOff", err)
	}
}
