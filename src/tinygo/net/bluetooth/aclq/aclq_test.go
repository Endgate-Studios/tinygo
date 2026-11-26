package aclq

import (
	"testing"

	"runtime/memory/arena"
	"tinygo/net/bluetooth/hci"
)

type sink struct{ got []byte }

func (s *sink) WriteHCI(p []byte) error {
	s.got = append(s.got, p[0])
	return nil
}

func TestOrderAndCredits(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 1024))
	var q Queue
	q.Init(mem, 64)
	flow := hci.NewFlow(1)
	w := &sink{}
	for i := byte(1); i <= 4; i++ {
		if err := q.Send([]byte{i, 0, 0, 0}, &flow, w); err != nil {
			t.Fatal(err)
		}
	}
	if string(w.got) != "\x01" || q.Len() != 3 || q.Bytes() != 12 {
		t.Fatalf("sent %v, queued %d packets and %d bytes", w.got, q.Len(), q.Bytes())
	}
	// Two buffers come back: packets 2 and 3 go, in order.
	flow.Update(completed(2))
	if err := q.Drain(&flow, w); err != nil || string(w.got) != "\x01\x02\x03" || q.Len() != 1 {
		t.Fatalf("after two credits: sent %v, queued %d, %v", w.got, q.Len(), err)
	}
	q.Reset()
	if q.Len() != 0 || q.Bytes() != 0 || mem.Stats().InUse != 0 {
		t.Errorf("after Reset: %d packets, %d bytes, memory in use %d", q.Len(), q.Bytes(), mem.Stats().InUse)
	}
}

func TestLimits(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 1024))
	var off Queue
	flow := hci.NewFlow(0)
	if err := off.Send([]byte{1}, &flow, &sink{}); err != ErrNoCredits {
		t.Errorf("queue off with no credit: %v", err)
	}
	var q Queue
	q.Init(mem, 10)
	if err := q.Send(make([]byte, 8), &flow, &sink{}); err != nil {
		t.Fatal(err)
	}
	if err := q.Send(make([]byte, 3), &flow, &sink{}); err != ErrFull {
		t.Errorf("over the byte limit: %v", err)
	}
	var many Queue
	many.Init(mem, 1000)
	var err error
	for i := 0; i <= Slots && err == nil; i++ {
		err = many.Send([]byte{1}, &flow, &sink{})
	}
	if err != ErrFull || many.Len() != Slots {
		t.Errorf("over the slot limit: %v, %d packets", err, many.Len())
	}
	q.Reset()
	many.Reset()
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use %d", mem.Stats().InUse)
	}
}

// completed is Number Of Completed Packets for one handle.
func completed(n uint16) hci.Event {
	return hci.Event{Code: hci.EvtNumCompletedPackets, Params: []byte{1, 0x40, 0x00, byte(n), byte(n >> 8)}}
}
