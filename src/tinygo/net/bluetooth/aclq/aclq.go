// Package aclq holds HCI ACL packets that wait for a free controller buffer (Core spec v5.4
// Vol 4 Part E 4.1.1). Each packet is a copy in its own block from the injected memory.
package aclq

import (
	"unsafe"

	"runtime/memory"
	"tinygo/net/bluetooth/hci"
)

// Memory is where the packet copies come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Writer sends one H4 packet to the controller.
type Writer interface {
	WriteHCI(packet []byte) error
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrFull      Error = "aclq: queue is full"
	ErrNoMemory  Error = "aclq: out of memory"
	ErrNoCredits Error = "aclq: controller has no free ACL buffer and the queue is off"
)

// Slots is the most packets that wait at the same time.
const Slots = 16

// Queue is a FIFO of ACL packets. Its zero value is off. SMP keys pass through it, so each block is
// zeroed before it is freed.
type Queue struct {
	mem   Memory
	slots [Slots][]byte
	head  int
	n     int
	bytes int
	limit int
}

// Init turns the queue on with at most limit bytes. A limit of 0 keeps it off.
func (q *Queue) Init(mem Memory, limit int) { q.mem, q.limit = mem, limit }

// Len returns the number of waiting packets, and Bytes their total size.
func (q *Queue) Len() int   { return q.n }
func (q *Queue) Bytes() int { return q.bytes }

// Send writes packet now when nothing waits and a buffer is free, and queues it otherwise, so
// packets keep their order.
func (q *Queue) Send(packet []byte, flow *hci.Flow, w Writer) error {
	if q.n == 0 && flow.CanSendACL() {
		flow.TookACL()
		return w.WriteHCI(packet)
	}
	if q.limit == 0 {
		return ErrNoCredits
	}
	return q.push(packet)
}

// Drain writes waiting packets while the controller has free buffers.
func (q *Queue) Drain(flow *hci.Flow, w Writer) error {
	for q.n > 0 && flow.CanSendACL() {
		if err := w.WriteHCI(q.slots[q.head]); err != nil {
			return err
		}
		flow.TookACL()
		q.pop()
	}
	return nil
}

// Reset zeroes and frees all waiting packets.
func (q *Queue) Reset() {
	for q.n > 0 {
		q.pop()
	}
	q.head = 0
}

func (q *Queue) push(pkt []byte) error {
	if q.n == Slots || q.bytes+len(pkt) > q.limit {
		return ErrFull
	}
	b := memory.MakeSlice[byte](q.mem, len(pkt))
	if b == nil {
		return ErrNoMemory
	}
	copy(b, pkt)
	q.slots[(q.head+q.n)%Slots] = b
	q.n++
	q.bytes += len(pkt)
	return nil
}

func (q *Queue) pop() {
	b := q.slots[q.head]
	q.bytes -= len(b)
	clear(b)
	q.mem.Free(unsafe.Pointer(unsafe.SliceData(b)))
	q.slots[q.head] = nil
	q.head = (q.head + 1) % Slots
	q.n--
}
