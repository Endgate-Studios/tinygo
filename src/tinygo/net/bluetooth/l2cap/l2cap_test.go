package l2cap

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"unsafe"

	"runtime/memory/arena"
	"tinygo/net/bluetooth/hci"
)

// checkedMemory is an arena heap that records whether each freed block was zeroed first.
type checkedMemory struct {
	*arena.Heap
	sizes      map[unsafe.Pointer]uintptr
	dirtyFrees int
}

func newCheckedMemory(t *testing.T) *checkedMemory {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 4096))
	if err != nil {
		t.Fatal(err)
	}
	return &checkedMemory{Heap: h, sizes: map[unsafe.Pointer]uintptr{}}
}

func (m *checkedMemory) Alloc(size, align uintptr) unsafe.Pointer {
	p := m.Heap.Alloc(size, align)
	if p != nil {
		m.sizes[p] = size
	}
	return p
}

func (m *checkedMemory) Free(p unsafe.Pointer) {
	for _, b := range unsafe.Slice((*byte)(p), m.sizes[p]) {
		if b != 0 {
			m.dirtyFrees++
			break
		}
	}
	delete(m.sizes, p)
	m.Heap.Free(p)
}

func (m *checkedMemory) check(t *testing.T) {
	t.Helper()
	if n := m.Heap.Stats().InUse; n != 0 || m.dirtyFrees != 0 {
		t.Errorf("memory in use %d, blocks freed without zeroing %d", n, m.dirtyFrees)
	}
}

type frame struct {
	handle, cid uint16
	payload     []byte
}

type frames struct{ got []frame }

func (f *frames) HandleFrame(handle, cid uint16, payload []byte) {
	f.got = append(f.got, frame{handle, cid, append([]byte(nil), payload...)})
}

// wire is an ACLWriter that keeps the packets.
type wire struct{ packets [][]byte }

func (w *wire) WriteACL(p []byte) error {
	w.packets = append(w.packets, append([]byte(nil), p...))
	return nil
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func acl(handle uint16, pb uint8, data []byte) hci.ACL {
	return hci.ACL{Handle: handle, PB: pb, Data: data}
}

func TestSingleFragment(t *testing.T) {
	mem := newCheckedMemory(t)
	got := &frames{}
	r, _ := NewReassembler(mem, got, 247)
	// ATT Exchange MTU Request in one packet: length 3, CID 4, opcode 0x02, MTU 247.
	if err := r.HandleACL(acl(0x40, hci.PBFirstFlushable, unhex(t, "0300 0400 02 f700"))); err != nil {
		t.Fatal(err)
	}
	if len(got.got) != 1 || got.got[0].cid != CIDATT || got.got[0].handle != 0x40 || !bytes.Equal(got.got[0].payload, unhex(t, "02f700")) {
		t.Fatalf("frames = %+v", got.got)
	}
	mem.check(t)
}

func TestSendAndReassemble(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		maxACL  int
		packets int
	}{
		{name: "fits in one packet", size: 20, maxACL: 27, packets: 1},
		{name: "exactly one packet", size: 23, maxACL: 27, packets: 1},
		{name: "one byte more", size: 24, maxACL: 27, packets: 2},
		{name: "100 bytes over 27", size: 100, maxACL: 27, packets: 4},
		{name: "empty payload", size: 0, maxACL: 27, packets: 1},
		{name: "large ACL buffers", size: 200, maxACL: 251, packets: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			payload := make([]byte, tt.size)
			for i := range payload {
				payload[i] = byte(i)
			}
			w := &wire{}
			if err := Send(w, mem, 0x41, CIDSMP, payload, tt.maxACL); err != nil {
				t.Fatal(err)
			}
			if len(w.packets) != tt.packets {
				t.Fatalf("%d packets, want %d", len(w.packets), tt.packets)
			}
			got := &frames{}
			r, _ := NewReassembler(mem, got, 512)
			for i, p := range w.packets {
				a, err := hci.ParseACL(p)
				if err != nil || len(a.Data) > tt.maxACL {
					t.Fatalf("packet %d: %v, %d data bytes", i, err, len(a.Data))
				}
				wantPB := uint8(hci.PBContinuing)
				if i == 0 {
					wantPB = hci.PBFirstNonFlushable
				}
				if a.PB != wantPB || a.Handle != 0x41 {
					t.Fatalf("packet %d: PB %d handle %#x", i, a.PB, a.Handle)
				}
				if err := r.HandleACL(a); err != nil {
					t.Fatal(err)
				}
			}
			if len(got.got) != 1 || got.got[0].cid != CIDSMP || !bytes.Equal(got.got[0].payload, payload) {
				t.Fatalf("reassembled %+v", got.got)
			}
			r.Close()
			mem.check(t)
		})
	}
}

func TestHeaderSplitAcrossFragments(t *testing.T) {
	mem := newCheckedMemory(t)
	got := &frames{}
	r, _ := NewReassembler(mem, got, 64)
	for _, p := range []hci.ACL{
		acl(0x40, hci.PBFirstFlushable, unhex(t, "04")),
		acl(0x40, hci.PBContinuing, unhex(t, "00 04")),
		acl(0x40, hci.PBContinuing, unhex(t, "00 0a01")),
		acl(0x40, hci.PBContinuing, unhex(t, "00ff")),
	} {
		if err := r.HandleACL(p); err != nil {
			t.Fatal(err)
		}
	}
	if len(got.got) != 1 || !bytes.Equal(got.got[0].payload, unhex(t, "0a0100ff")) {
		t.Fatalf("frames = %+v", got.got)
	}
	mem.check(t)
}

func TestTwoConnectionsInterleaved(t *testing.T) {
	mem := newCheckedMemory(t)
	got := &frames{}
	r, _ := NewReassembler(mem, got, 64)
	steps := []hci.ACL{
		acl(1, hci.PBFirstFlushable, unhex(t, "0400 0400 aa")),
		acl(2, hci.PBFirstFlushable, unhex(t, "0200 0600 bb")),
		acl(1, hci.PBContinuing, unhex(t, "aaaaaa")),
		acl(2, hci.PBContinuing, unhex(t, "bb")),
	}
	for _, p := range steps {
		if err := r.HandleACL(p); err != nil {
			t.Fatal(err)
		}
	}
	// Connection 1 gets its last fragment first, so its frame is complete first.
	if len(got.got) != 2 || got.got[0].handle != 1 || got.got[1].handle != 2 ||
		!bytes.Equal(got.got[0].payload, unhex(t, "aaaaaaaa")) || !bytes.Equal(got.got[1].payload, unhex(t, "bbbb")) {
		t.Fatalf("frames = %+v", got.got)
	}
	mem.check(t)
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name    string
		steps   []hci.ACL
		wantErr error
	}{
		{name: "continuation without a start", steps: []hci.ACL{acl(1, hci.PBContinuing, unhex(t, "aa"))}, wantErr: ErrUnexpected},
		{name: "frame too long", steps: []hci.ACL{acl(1, hci.PBFirstFlushable, unhex(t, "4100 0400"))}, wantErr: ErrTooLong},
		{name: "fragments longer than the frame", steps: []hci.ACL{acl(1, hci.PBFirstFlushable, unhex(t, "0200 0400 aa")), acl(1, hci.PBContinuing, unhex(t, "bbcc"))}, wantErr: ErrOverrun},
		{name: "too many connections", steps: []hci.ACL{
			acl(1, hci.PBFirstFlushable, unhex(t, "0800")), acl(2, hci.PBFirstFlushable, unhex(t, "0800")),
			acl(3, hci.PBFirstFlushable, unhex(t, "0800")), acl(4, hci.PBFirstFlushable, unhex(t, "0800")),
			acl(5, hci.PBFirstFlushable, unhex(t, "0800")),
		}, wantErr: ErrTooManyConns},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			got := &frames{}
			r, _ := NewReassembler(mem, got, 64)
			var err error
			for _, p := range tt.steps {
				if err = r.HandleACL(p); err != nil {
					break
				}
			}
			if err != tt.wantErr {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			r.Close()
			mem.check(t)
		})
	}
}

func TestNewStartDropsUnfinishedFrame(t *testing.T) {
	mem := newCheckedMemory(t)
	got := &frames{}
	r, _ := NewReassembler(mem, got, 64)
	r.HandleACL(acl(1, hci.PBFirstFlushable, unhex(t, "0800 0400 aabb")))
	r.HandleACL(acl(1, hci.PBFirstFlushable, unhex(t, "0100 0400 cc")))
	if len(got.got) != 1 || !bytes.Equal(got.got[0].payload, []byte{0xcc}) {
		t.Fatalf("frames = %+v", got.got)
	}
	r.HandleACL(acl(1, hci.PBFirstFlushable, unhex(t, "0800 0400 aabb")))
	r.Reset(1)
	if err := r.HandleACL(acl(1, hci.PBContinuing, unhex(t, "cc"))); err != ErrUnexpected {
		t.Errorf("continuation after Reset error = %v, want ErrUnexpected", err)
	}
	mem.check(t)
}

func TestSignaling(t *testing.T) {
	buf := make([]byte, 16)
	n, _ := PutCommandReject(buf, 7)
	if !bytes.Equal(buf[:n], unhex(t, "01 07 0200 0000")) {
		t.Errorf("Command Reject = %x", buf[:n])
	}
	p := ConnParams{IntervalMin: 6, IntervalMax: 12, Latency: 0, Timeout: 500}
	n, _ = PutConnParamUpdateReq(buf, 1, p)
	if !bytes.Equal(buf[:n], unhex(t, "12 01 0800 0600 0c00 0000 f401")) {
		t.Errorf("Conn Param Update Request = %x", buf[:n])
	}
	s, err := ParseSignal(buf[:n])
	if err != nil || s.Code != SigConnParamUpdateReq || s.Identifier != 1 {
		t.Fatalf("ParseSignal = %+v, %v", s, err)
	}
	if got, err := ParseConnParamUpdateReq(s); err != nil || got != p {
		t.Errorf("ParseConnParamUpdateReq = %+v, %v", got, err)
	}
	n, _ = PutConnParamUpdateRsp(buf, 1, ParamsAccepted)
	if !bytes.Equal(buf[:n], unhex(t, "13 01 0200 0000")) {
		t.Errorf("Conn Param Update Response = %x", buf[:n])
	}
	if _, err := ParseSignal(unhex(t, "12 01 0800 0600")); err != ErrShort {
		t.Errorf("truncated signal error = %v", err)
	}
}

func TestConnParamsValid(t *testing.T) {
	tests := []struct {
		name string
		p    ConnParams
		want bool
	}{
		{name: "typical", p: ConnParams{IntervalMin: 24, IntervalMax: 40, Latency: 0, Timeout: 200}, want: true},
		{name: "interval below 7.5 ms", p: ConnParams{IntervalMin: 5, IntervalMax: 40, Timeout: 200}},
		{name: "min above max", p: ConnParams{IntervalMin: 50, IntervalMax: 40, Timeout: 200}},
		{name: "latency 500", p: ConnParams{IntervalMin: 6, IntervalMax: 6, Latency: 500, Timeout: 3200}},
		// 100 ms timeout against (1 + 4) * 50 ms * 2 = 500 ms.
		{name: "timeout too short", p: ConnParams{IntervalMin: 40, IntervalMax: 40, Latency: 4, Timeout: 10}},
		{name: "edge of the timeout rule", p: ConnParams{IntervalMin: 40, IntervalMax: 40, Latency: 0, Timeout: 11}, want: true},
	}
	for _, tt := range tests {
		if got := tt.p.Valid(); got != tt.want {
			t.Errorf("%s: Valid = %v, want %v", tt.name, got, tt.want)
		}
	}
}
