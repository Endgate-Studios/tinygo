package central

import (
	"bytes"
	"encoding/binary"
	"testing"

	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
)

const handle = 0x0041

var peerAddr = hci.Addr{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6}

type l2frame struct {
	cid  uint16
	data []byte
}

// link is the controller and the peripheral. Commands collect in cmds, and ACL data from the
// central is reassembled into frames.
type testLink struct {
	t      *testing.T
	c      *Central
	cmds   []hci.Command
	rx     *l2cap.Reassembler
	frames []l2frame
	up     []hci.Addr
	down   []uint8
	done   []error
	notes  [][]byte
}

func (l *testLink) WriteHCI(b []byte) error {
	switch b[0] {
	case hci.TypeCommand:
		cmd, err := hci.ParseCommand(append([]byte(nil), b...))
		if err != nil {
			return err
		}
		l.cmds = append(l.cmds, cmd)
	case hci.TypeACL:
		a, err := hci.ParseACL(b)
		if err != nil {
			return err
		}
		return l.rx.HandleACL(a)
	}
	return nil
}

func (l *testLink) HandleFrame(_, cid uint16, p []byte) {
	l.frames = append(l.frames, l2frame{cid, append([]byte(nil), p...)})
}

// WriteACL delivers ACL data from the peripheral to the central.
func (l *testLink) WriteACL(p []byte) error { return l.c.HandleHCI(p, 0) }

func (l *testLink) Connected(_ uint16, peer hci.Addr) { l.up = append(l.up, peer) }
func (l *testLink) Disconnected(reason uint8)         { l.down = append(l.down, reason) }
func (l *testLink) ProcedureDone(err error)           { l.done = append(l.done, err) }
func (l *testLink) Notification(_ uint16, v []byte, _ bool) {
	l.notes = append(l.notes, append([]byte(nil), v...))
}

func newLink(t *testing.T) *testLink {
	t.Helper()
	l := &testLink{t: t}
	cfg := testConfig(ScanConfig{Interval: 0x60, Window: 0x30})
	cfg.Conn, cfg.Done, cfg.Notify = l, l, l
	c, err := New(newMem(), l, cfg)
	if err != nil {
		t.Fatal(err)
	}
	l.c = c
	l.rx, _ = l2cap.NewReassembler(newMem(), l, 512)
	return l
}

func (l *testLink) event(code uint8, params []byte) error {
	var b [260]byte
	n, _ := hci.PutEvent(b[:], code, params)
	return l.c.HandleHCI(b[:n], 0)
}

// connComplete is LE Connection Complete as central, with the given status.
func connComplete(status uint8) []byte {
	p := make([]byte, 19)
	p[0], p[1] = hci.SubConnectionComplete, status
	binary.LittleEndian.PutUint16(p[2:], handle)
	p[4], p[5] = 0, hci.AddrPublic // role central
	copy(p[6:12], peerAddr[:])
	return p
}

func (l *testLink) connect() {
	l.t.Helper()
	if err := l.c.Connect(&hci.ConnParams{ScanInterval: 0x60, ScanWindow: 0x30, PeerAddr: peerAddr, IntervalMin: 24, IntervalMax: 40, SupervisionTimeout: 200}); err != nil {
		l.t.Fatal(err)
	}
	if len(l.cmds) == 0 || l.cmds[len(l.cmds)-1].Op != hci.OpLECreateConnection {
		l.t.Fatalf("commands %+v, want LE Create Connection", l.cmds)
	}
	if err := l.event(hci.EvtCommandStatus, []byte{0, 1, 0x0d, 0x20}); err != nil || l.c.ConnState() != ConnConnecting {
		l.t.Fatalf("command status: %v, state %d", err, l.c.ConnState())
	}
	if err := l.event(hci.EvtLEMeta, connComplete(0)); err != nil || l.c.ConnState() != ConnConnected {
		l.t.Fatalf("connection complete: %v, state %d", err, l.c.ConnState())
	}
}

func (l *testLink) send(cid uint16, p []byte) {
	l.t.Helper()
	if err := l2cap.Send(l, newMem(), handle, cid, p, 27); err != nil {
		l.t.Fatal(err)
	}
}

func (l *testLink) pop(cid uint16) []byte {
	l.t.Helper()
	if len(l.frames) == 0 || l.frames[0].cid != cid {
		l.t.Fatalf("frames %+v, want one on channel %#x", l.frames, cid)
	}
	f := l.frames[0].data
	l.frames = l.frames[1:]
	return f
}

func TestConnectAndDisconnect(t *testing.T) {
	l := newLink(t)
	l.connect()
	if len(l.up) != 1 || l.up[0] != peerAddr || l.c.Handle() != handle {
		t.Errorf("connected %v, handle %#x", l.up, l.c.Handle())
	}
	if err := l.c.Connect(&hci.ConnParams{}); err != ErrBusy {
		t.Errorf("second Connect: %v", err)
	}
	if err := l.c.Disconnect(0x13); err != nil || l.cmds[len(l.cmds)-1].Op != hci.OpDisconnect {
		t.Fatalf("disconnect: %v, %+v", err, l.cmds)
	}
	l.event(hci.EvtCommandStatus, []byte{0, 1, 0x06, 0x04})
	l.event(hci.EvtDisconnectionComplete, []byte{0, byte(handle), byte(handle >> 8), 0x16})
	if l.c.ConnState() != ConnIdle || len(l.down) != 1 || l.down[0] != 0x16 {
		t.Errorf("state %d, disconnected %v", l.c.ConnState(), l.down)
	}
}

func TestConnectFailures(t *testing.T) {
	l := newLink(t)
	l.c.Connect(&hci.ConnParams{PeerAddr: peerAddr})
	// 0x0c is Command Disallowed (Core spec v5.4 Vol 1 Part F 2.13).
	if err := l.event(hci.EvtCommandStatus, []byte{0x0c, 1, 0x0d, 0x20}); err != ErrController || l.c.ConnState() != ConnIdle || l.c.Status() != 0x0c {
		t.Errorf("rejected create: %v, state %d, status %#x", err, l.c.ConnState(), l.c.Status())
	}
	l.c.Connect(&hci.ConnParams{PeerAddr: peerAddr})
	l.event(hci.EvtCommandStatus, []byte{0, 1, 0x0d, 0x20})
	// 0x3e is Connection Failed to be Established (Vol 1 Part F 2.62).
	if err := l.event(hci.EvtLEMeta, connComplete(0x3e)); err != ErrController || l.c.ConnState() != ConnIdle || len(l.up) != 0 {
		t.Errorf("failed connection: %v, state %d, connected %v", err, l.c.ConnState(), l.up)
	}

	s := newLink(t)
	s.c.Scan(&reports{})
	if err := s.c.Connect(&hci.ConnParams{}); err != ErrBusy {
		t.Errorf("connect while scan starts: %v", err)
	}
}

func TestAttributeTraffic(t *testing.T) {
	l := newLink(t)
	l.connect()

	// The peripheral exchanges the MTU, and the central answers as a server.
	l.send(l2cap.CIDATT, []byte{0x02, 0xf7, 0x00})
	if rsp := l.pop(l2cap.CIDATT); !bytes.Equal(rsp, []byte{0x03, 64, 0}) || l.c.Client().MTU() != 64 {
		t.Errorf("MTU response % x, client MTU %d", rsp, l.c.Client().MTU())
	}
	// Other requests are not supported, and commands get no answer.
	l.send(l2cap.CIDATT, []byte{0x0a, 0x03, 0x00})
	if rsp := l.pop(l2cap.CIDATT); !bytes.Equal(rsp, []byte{0x01, 0x0a, 0, 0, 0x06}) {
		t.Errorf("read response % x", rsp)
	}
	l.send(l2cap.CIDATT, []byte{0x52, 0x03, 0x00, 1})
	if len(l.frames) != 0 {
		t.Errorf("answer to a command % x", l.frames)
	}

	// The GATT client discovers services, and the peripheral has none.
	if err := l.c.Client().DiscoverServices(&found{}); err != nil {
		t.Fatal(err)
	}
	if req := l.pop(l2cap.CIDATT); req[0] != 0x10 {
		t.Fatalf("discovery request % x", req)
	}
	l.send(l2cap.CIDATT, []byte{0x01, 0x10, 0x01, 0x00, 0x0a})
	if len(l.done) != 1 || l.done[0] != nil {
		t.Errorf("procedure results %v", l.done)
	}
	l.send(l2cap.CIDATT, []byte{0x1b, 0x03, 0x00, 42})
	if len(l.notes) != 1 || l.notes[0][0] != 42 {
		t.Errorf("notifications %v", l.notes)
	}
}

func TestSignaling(t *testing.T) {
	l := newLink(t)
	l.connect()
	var req [16]byte
	n, _ := l2cap.PutConnParamUpdateReq(req[:], 5, l2cap.ConnParams{IntervalMin: 6, IntervalMax: 12, Timeout: 100})
	l.send(l2cap.CIDSignaling, req[:n])
	if rsp := l.pop(l2cap.CIDSignaling); rsp[0] != l2cap.SigConnParamUpdateRsp || rsp[1] != 5 || binary.LittleEndian.Uint16(rsp[4:]) != l2cap.ParamsRejected {
		t.Errorf("parameter update response % x", rsp)
	}
	l.send(l2cap.CIDSignaling, []byte{0x14, 0x07, 0x00, 0x00})
	if rsp := l.pop(l2cap.CIDSignaling); rsp[0] != l2cap.SigCommandReject || rsp[1] != 7 {
		t.Errorf("reject % x", rsp)
	}
}

type found struct{ services int }

func (f *found) Service(uint16, uint16, att.UUID) { f.services++ }

// With one controller buffer, the second answer waits in the queue until the buffer comes back.
func TestQueue(t *testing.T) {
	l := &testLink{t: t}
	cfg := testConfig(ScanConfig{Interval: 0x60, Window: 0x30})
	cfg.ACLBuffers, cfg.QueueBytes = 1, 64
	c, err := New(newMem(), l, cfg)
	if err != nil {
		t.Fatal(err)
	}
	l.c = c
	l.rx, _ = l2cap.NewReassembler(newMem(), l, 512)
	l.connect()
	l.send(l2cap.CIDATT, []byte{0x02, 0x40, 0x00}) // MTU answer takes the only buffer
	l.send(l2cap.CIDATT, []byte{0x0a, 0x03, 0x00}) // the error answer waits
	if len(l.frames) != 1 || c.queue.Len() != 1 {
		t.Fatalf("frames %d, queued %d, want 1 and 1", len(l.frames), c.queue.Len())
	}
	l.event(hci.EvtNumCompletedPackets, []byte{1, byte(handle), byte(handle >> 8), 1, 0})
	l.pop(l2cap.CIDATT)
	if rsp := l.pop(l2cap.CIDATT); rsp[0] != 0x01 || c.queue.Len() != 0 {
		t.Errorf("queued answer % x, queued %d", rsp, c.queue.Len())
	}
	l.send(l2cap.CIDATT, []byte{0x0a, 0x03, 0x00})
	l.event(hci.EvtDisconnectionComplete, []byte{0, byte(handle), byte(handle >> 8), 0x13})
	if c.queue.Len() != 0 {
		t.Errorf("queued %d after disconnection", c.queue.Len())
	}
}

type policy struct {
	ok   bool
	seen int
}

func (p *policy) AcceptParams(l2cap.ConnParams) bool {
	p.seen++
	return p.ok
}

func TestParamUpdate(t *testing.T) {
	pol := &policy{ok: true}
	l := &testLink{t: t}
	cfg := testConfig(ScanConfig{Interval: 0x60, Window: 0x30})
	cfg.Params = pol
	c, err := New(newMem(), l, cfg)
	if err != nil {
		t.Fatal(err)
	}
	l.c = c
	l.rx, _ = l2cap.NewReassembler(newMem(), l, 512)
	l.connect()
	request := func(id uint8, p l2cap.ConnParams) uint16 {
		t.Helper()
		var req [16]byte
		n, _ := l2cap.PutConnParamUpdateReq(req[:], id, p)
		l.send(l2cap.CIDSignaling, req[:n])
		rsp := l.pop(l2cap.CIDSignaling)
		if rsp[0] != l2cap.SigConnParamUpdateRsp || rsp[1] != id {
			t.Fatalf("response % x", rsp)
		}
		return binary.LittleEndian.Uint16(rsp[4:])
	}
	cmds := len(l.cmds)
	if r := request(1, l2cap.ConnParams{IntervalMin: 6, IntervalMax: 12, Latency: 1, Timeout: 100}); r != l2cap.ParamsAccepted {
		t.Fatalf("valid request: result %d", r)
	}
	if len(l.cmds) != cmds+1 || l.cmds[cmds].Op != hci.OpLEConnectionUpdate || binary.LittleEndian.Uint16(l.cmds[cmds].Params[2:]) != 6 {
		t.Fatalf("commands %+v, want LE Connection Update", l.cmds[cmds:])
	}
	l.event(hci.EvtLEMeta, []byte{hci.SubConnectionUpdate, 0, byte(handle), byte(handle >> 8), 12, 0, 1, 0, 100, 0})
	if got := c.Params(); got != (ConnParams{Interval: 12, Latency: 1, Timeout: 100}) {
		t.Errorf("params after update %+v", got)
	}
	// The spec ranges are checked before the policy sees a request.
	if r := request(2, l2cap.ConnParams{IntervalMin: 40, IntervalMax: 40, Latency: 4, Timeout: 10}); r != l2cap.ParamsRejected || pol.seen != 1 {
		t.Errorf("invalid request: result %d, policy calls %d", r, pol.seen)
	}
	pol.ok = false
	if r := request(3, l2cap.ConnParams{IntervalMin: 6, IntervalMax: 12, Timeout: 100}); r != l2cap.ParamsRejected || len(l.cmds) != cmds+1 {
		t.Errorf("policy says no: result %d, commands %d", r, len(l.cmds)-cmds)
	}
}
