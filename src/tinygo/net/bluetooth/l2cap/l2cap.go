// Package l2cap is LE L2CAP over HCI ACL (Core spec v5.4 Vol 3 Part A): frame reassembly,
// fragmentation and the LE signaling channel. Reassembly buffers come from an injected allocator that can free.
package l2cap

import (
	"encoding/binary"
	"unsafe"

	"runtime/memory"
	"tinygo/net/bluetooth/hci"
)

// Fixed channel IDs on an LE link (Core spec v5.4 Vol 3 Part A section 2.1).
const (
	CIDATT       = 0x0004
	CIDSignaling = 0x0005
	CIDSMP       = 0x0006
)

// HeaderLen is the basic L2CAP header: length and channel ID (Core spec v5.4 Vol 3 Part A 3.1).
const HeaderLen = 4

// MaxConns is the number of connections that can reassemble frames at the same time.
const MaxConns = 4

// Memory is where reassembly and send buffers come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// FrameHandler receives complete L2CAP frames. The payload is valid only during the call.
type FrameHandler interface {
	HandleFrame(handle, cid uint16, payload []byte)
}

// ACLWriter sends one H4 ACL packet to the controller.
type ACLWriter interface {
	WriteACL(packet []byte) error
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilDependency Error = "l2cap: memory and handler cannot be nil"
	ErrNoMemory      Error = "l2cap: out of memory"
	ErrTooLong       Error = "l2cap: frame is longer than the maximum"
	ErrUnexpected    Error = "l2cap: continuation fragment without a start"
	ErrOverrun       Error = "l2cap: fragments are longer than the frame"
	ErrTooManyConns  Error = "l2cap: too many connections are reassembling"
	ErrMTU           Error = "l2cap: ACL size must be at least 5 bytes"
	ErrShort         Error = "l2cap: frame is too short"
	ErrClosed        Error = "l2cap: reassembler is closed"
)

type slot struct {
	used   bool
	handle uint16
	head   [HeaderLen]byte // header bytes, while they arrive
	nHead  int
	buf    []byte // payload, from Memory once the length is known
	got    int
}

// Reassembler joins ACL fragments into L2CAP frames for each connection.
type Reassembler struct {
	mem      Memory
	handler  FrameHandler
	maxFrame int
	slots    [MaxConns]slot
	closed   bool
}

// NewReassembler returns a reassembler that accepts frame payloads of at most maxFrame bytes.
func NewReassembler(mem Memory, handler FrameHandler, maxFrame int) (*Reassembler, error) {
	if mem == nil || handler == nil {
		return nil, ErrNilDependency
	}
	return &Reassembler{mem: mem, handler: handler, maxFrame: maxFrame}, nil
}

// HandleACL takes one ACL packet from the controller.
func (r *Reassembler) HandleACL(a hci.ACL) error {
	if r.closed {
		return ErrClosed
	}
	if a.PB == hci.PBContinuing {
		s := r.find(a.Handle)
		if s == nil {
			return ErrUnexpected
		}
		return r.add(s, a.Data)
	}
	s := r.find(a.Handle)
	if s != nil {
		r.drop(s) // a new start ends the unfinished frame
	} else if s = r.free(); s == nil {
		return ErrTooManyConns
	}
	s.used, s.handle = true, a.Handle
	return r.add(s, a.Data)
}

// Reset drops an unfinished frame of a connection, for example after a disconnection.
func (r *Reassembler) Reset(handle uint16) {
	if s := r.find(handle); s != nil {
		r.drop(s)
	}
}

// Close frees all unfinished frames.
func (r *Reassembler) Close() {
	for i := range r.slots {
		if r.slots[i].used {
			r.drop(&r.slots[i])
		}
	}
	r.closed = true
}

func (r *Reassembler) add(s *slot, data []byte) error {
	for s.nHead < HeaderLen && len(data) > 0 {
		s.head[s.nHead] = data[0]
		s.nHead++
		data = data[1:]
	}
	if s.nHead < HeaderLen {
		return nil
	}
	n := int(binary.LittleEndian.Uint16(s.head[:]))
	if s.buf == nil {
		if n > r.maxFrame {
			r.drop(s)
			return ErrTooLong
		}
		if n > 0 {
			if s.buf = memory.MakeSlice[byte](r.mem, n); s.buf == nil {
				r.drop(s)
				return ErrNoMemory
			}
		}
	}
	if s.got+len(data) > n {
		r.drop(s)
		return ErrOverrun
	}
	s.got += copy(s.buf[s.got:], data)
	if s.got < n {
		return nil
	}
	cid := binary.LittleEndian.Uint16(s.head[2:])
	r.handler.HandleFrame(s.handle, cid, s.buf[:n])
	r.drop(s)
	return nil
}

func (r *Reassembler) find(handle uint16) *slot {
	for i := range r.slots {
		if r.slots[i].used && r.slots[i].handle == handle {
			return &r.slots[i]
		}
	}
	return nil
}

func (r *Reassembler) free() *slot {
	for i := range r.slots {
		if !r.slots[i].used {
			return &r.slots[i]
		}
	}
	return nil
}

// drop frees the buffer of a slot and empties it.
func (r *Reassembler) drop(s *slot) {
	if s.buf != nil {
		clear(s.buf)
		memory.FreeSlice(r.mem, s.buf)
	}
	*s = slot{}
}

// Send splits an L2CAP frame into ACL packets of at most maxACL data bytes and writes them to w.
// maxACL is the LE ACL data packet length from HCI_LE_Read_Buffer_Size, 27 on the smallest controllers.
func Send(w ACLWriter, mem Memory, handle, cid uint16, payload []byte, maxACL int) error {
	if maxACL < HeaderLen+1 {
		return ErrMTU
	}
	if len(payload) > 0xffff {
		return ErrTooLong
	}
	pkt := memory.MakeSlice[byte](mem, 5+maxACL)
	if pkt == nil {
		return ErrNoMemory
	}
	defer release(mem, pkt) // SMP key distribution passes through here, so the buffer is zeroed

	// The first packet holds the L2CAP header and the start of the payload.
	var head [HeaderLen]byte
	binary.LittleEndian.PutUint16(head[0:], uint16(len(payload)))
	binary.LittleEndian.PutUint16(head[2:], cid)
	first := min(len(payload), maxACL-HeaderLen)
	n := copy(pkt[5:], head[:])
	n += copy(pkt[5+n:], payload[:first])
	if err := writePacket(w, pkt, handle, hci.PBFirstNonFlushable, n); err != nil {
		return err
	}
	for rest := payload[first:]; len(rest) > 0; {
		n := copy(pkt[5:5+maxACL], rest)
		rest = rest[n:]
		if err := writePacket(w, pkt, handle, hci.PBContinuing, n); err != nil {
			return err
		}
	}
	return nil
}

func release(mem Memory, b []byte) {
	clear(b)
	mem.Free(unsafe.Pointer(unsafe.SliceData(b)))
}

// writePacket fills the H4 and ACL headers in front of n data bytes already in pkt[5:].
func writePacket(w ACLWriter, pkt []byte, handle uint16, pb uint8, n int) error {
	pkt[0] = hci.TypeACL
	binary.LittleEndian.PutUint16(pkt[1:], handle&0x0fff|uint16(pb)<<12)
	binary.LittleEndian.PutUint16(pkt[3:], uint16(n))
	return w.WriteACL(pkt[:5+n])
}
