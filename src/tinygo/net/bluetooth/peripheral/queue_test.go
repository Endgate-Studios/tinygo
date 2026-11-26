package peripheral

import (
	"testing"

	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

// queueHarness has one controller buffer and a queue of 64 bytes.
func queueHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, &store{})
	h.p.cfg.ACLBuffers = 1
	h.p.queue.Init(h.mem, 64)
	h.start()
	h.connect(smp.Address{Addr: [6]byte{1, 2, 3, 4, 5, 6}})
	return h
}

var (
	mtuReq   = []byte{0x02, 0x40, 0x00}
	readReq  = []byte{0x0a, 0x03, 0x00}
	oneFreed = []byte{1, byte(conn), byte(conn >> 8), 1, 0}
)

func TestQueueKeepsOrder(t *testing.T) {
	h := queueHarness(t)
	inUse := h.mem.Stats().InUse
	h.send(l2cap.CIDATT, mtuReq)  // takes the only buffer
	h.send(l2cap.CIDATT, readReq) // waits in the queue
	h.pop(l2cap.CIDATT, 0x03)
	if len(h.frames) != 0 || h.p.queue.Len() != 1 {
		t.Fatalf("frames %d, queued %d, want 0 and 1", len(h.frames), h.p.queue.Len())
	}
	if h.mem.Stats().InUse <= inUse {
		t.Error("the queued packet does not use the injected memory")
	}
	h.event(hci.EvtNumCompletedPackets, oneFreed)
	if rsp := h.pop(l2cap.CIDATT, 0x0b); rsp[1] != 77 {
		t.Errorf("queued read response % x", rsp)
	}
	if h.p.queue.Len() != 0 || h.mem.Stats().InUse != inUse {
		t.Errorf("queued %d, memory in use %d, want 0 and %d", h.p.queue.Len(), h.mem.Stats().InUse, inUse)
	}
}

func TestQueueLimitAndDisconnect(t *testing.T) {
	h := queueHarness(t)
	inUse := h.mem.Stats().InUse
	h.send(l2cap.CIDATT, mtuReq)
	var err error
	for i := 0; i < 20 && err == nil; i++ {
		err = l2cap.Send(h, h.cmem, conn, l2cap.CIDATT, readReq, 27)
	}
	if err != ErrQueueFull {
		t.Fatalf("queue over its limit: %v", err)
	}
	if h.p.queue.Bytes() > 64 {
		t.Errorf("queue holds %d bytes, limit 64", h.p.queue.Bytes())
	}
	h.event(hci.EvtDisconnectionComplete, []byte{0, byte(conn), byte(conn >> 8), 0x13})
	if h.p.queue.Len() != 0 || h.mem.Stats().InUse != inUse {
		t.Errorf("after disconnection: queued %d, memory in use %d, want 0 and %d", h.p.queue.Len(), h.mem.Stats().InUse, inUse)
	}
}
