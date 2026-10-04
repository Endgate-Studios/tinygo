package att

import (
	"bytes"
	"testing"

	"runtime/memory/arena"
)

type toServer struct{ got [][]byte }

func (s *toServer) SendATT(pdu []byte) error {
	s.got = append(s.got, append([]byte(nil), pdu...))
	return nil
}

type found struct {
	services [][3]uint16 // start, end, 16-bit UUID or 0
	chars    [][4]uint16 // decl, props, value, 16-bit UUID or 0
	descs    [][2]uint16
	value    []byte
	offsets  []uint16
	notes    []string
}

func short(u UUID) uint16 {
	s, _ := u.Short()
	return s
}

func (f *found) Service(start, end uint16, u UUID) {
	f.services = append(f.services, [3]uint16{start, end, short(u)})
}
func (f *found) Characteristic(decl uint16, props uint8, value uint16, u UUID) {
	f.chars = append(f.chars, [4]uint16{decl, uint16(props), value, short(u)})
}
func (f *found) Descriptor(h uint16, u UUID) { f.descs = append(f.descs, [2]uint16{h, short(u)}) }
func (f *found) ReadPart(_, offset uint16, data []byte) {
	f.offsets = append(f.offsets, offset)
	f.value = append(f.value, data...)
}
func (f *found) Notification(h uint16, v []byte, ind bool) {
	kind := "n"
	if ind {
		kind = "i"
	}
	f.notes = append(f.notes, kind+string(rune('0'+h))+":"+string(v))
}

type link struct {
	f   *fixture
	c   *Client
	cm  *arena.Heap
	out *toServer
	got *found
}

func newLink(t *testing.T, maxMTU uint16) *link {
	t.Helper()
	f := newFixture(t)
	cm, _ := arena.NewHeap(make([]byte, 1024))
	l := &link{f: f, cm: cm, out: &toServer{}, got: &found{}}
	c, err := NewClient(cm, l.out, l.got, maxMTU)
	if err != nil {
		t.Fatal(err)
	}
	l.c = c
	return l
}

// run moves PDUs between the client and the server until the procedure ends, and returns its result.
func (l *link) run(t *testing.T, start error) error {
	t.Helper()
	if start != nil {
		return start
	}
	for i := 0; i < 100; i++ {
		if len(l.out.got) == 0 {
			t.Fatal("client sent no request")
		}
		req := l.out.got[0]
		l.out.got = l.out.got[1:]
		l.f.cl.got = nil
		if err := l.f.srv.Handle(req); err != nil {
			t.Fatal(err)
		}
		for _, rsp := range l.f.cl.got {
			if !ForClient(rsp) {
				t.Fatalf("response % x does not route to the client", rsp)
			}
			if done, err := l.c.Handle(rsp); done {
				return err
			}
		}
	}
	t.Fatal("procedure did not end")
	return nil
}

func TestClientAgainstServer(t *testing.T) {
	l := newLink(t, 64)
	if err := l.run(t, l.c.ExchangeMTU()); err != nil || l.c.MTU() != 64 || l.f.srv.MTU() != 64 {
		t.Fatalf("MTU exchange: err %v, client %d, server %d", err, l.c.MTU(), l.f.srv.MTU())
	}
	if err := l.run(t, l.c.DiscoverServices(l.got)); err != nil {
		t.Fatal(err)
	}
	wantSvc := [][3]uint16{{1, 3, 0x1800}, {4, 7, 0x180f}, {8, 10, 0}}
	if !equal(l.got.services, wantSvc) {
		t.Errorf("services %v, want %v", l.got.services, wantSvc)
	}
	if err := l.run(t, l.c.DiscoverCharacteristics(4, 7, l.got)); err != nil {
		t.Fatal(err)
	}
	if want := [][4]uint16{{5, PropRead | PropNotify, 6, 0x2a19}}; !equal(l.got.chars, want) {
		t.Errorf("characteristics %v, want %v", l.got.chars, want)
	}
	if err := l.run(t, l.c.DiscoverDescriptors(7, 7, l.got)); err != nil {
		t.Fatal(err)
	}
	if want := [][2]uint16{{7, 0x2902}}; !equal(l.got.descs, want) {
		t.Errorf("descriptors %v, want %v", l.got.descs, want)
	}
	// All characteristics, so discovery takes more than one request and ends at 0xffff.
	l.got.chars = nil
	if err := l.run(t, l.c.DiscoverCharacteristics(1, 0xffff, l.got)); err != nil || len(l.got.chars) != 3 {
		t.Errorf("all characteristics: err %v, %v", err, l.got.chars)
	}

	long := bytes.Repeat([]byte("0123456789"), 15) // 150 bytes, 3 parts at MTU 64
	if err := l.run(t, l.c.Write(10, long[:61])); err != nil {
		t.Fatal(err)
	}
	l.f.store.value = long
	if err := l.run(t, l.c.Read(10, l.got)); err != nil || !bytes.Equal(l.got.value, long) {
		t.Fatalf("long read: err %v, got %q", err, l.got.value)
	}
	if want := []uint16{0, 63, 126}; !equal(l.got.offsets, want) {
		t.Errorf("read offsets %v, want %v", l.got.offsets, want)
	}
	// A value of exactly MTU-1 bytes needs one more Read Blob, which returns an empty part.
	l.got.value, l.got.offsets = nil, nil
	l.f.store.value = long[:63]
	if err := l.run(t, l.c.Read(10, l.got)); err != nil || len(l.got.value) != 63 {
		t.Errorf("read of MTU-1 bytes: err %v, %d bytes", err, len(l.got.value))
	}

	if err := l.run(t, l.c.Subscribe(7, true, false)); err != nil {
		t.Fatal(err)
	}
	l.f.cl.got = nil
	if err := l.f.srv.Notify(6, []byte("Z")); err != nil {
		t.Fatal(err)
	}
	if done, err := l.c.Handle(l.f.cl.got[0]); done || err != nil || len(l.got.notes) != 1 || l.got.notes[0] != "n6:Z" {
		t.Errorf("notification: done %v, err %v, %v", done, err, l.got.notes)
	}

	if err := l.run(t, l.c.Write(2, []byte("x"))); err != ErrResponse || l.c.ErrCode() != CodeWriteNotPermitted {
		t.Errorf("write to a read-only value: err %v, code %#x", err, l.c.ErrCode())
	}
	if err := l.c.WriteCommand(10, []byte("cmd")); err != nil {
		t.Fatal(err)
	}
	l.f.srv.Handle(l.out.got[0])
	if string(l.f.store.value) != "cmd" {
		t.Errorf("write command stored %q", l.f.store.value)
	}
	if n := l.cm.Stats().InUse; n != 0 {
		t.Errorf("client memory in use %d", n)
	}
}

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestClientBadResponses(t *testing.T) {
	tests := []struct {
		name  string
		start func(c *Client, h *found) error
		rsp   string
		want  error
	}{
		{name: "wrong opcode", start: func(c *Client, h *found) error { return c.DiscoverServices(h) }, rsp: "0b 00", want: ErrInvalidPDU},
		{name: "service length 5", start: func(c *Client, h *found) error { return c.DiscoverServices(h) }, rsp: "11 05 0100 0300 00", want: ErrInvalidPDU},
		{name: "service before start", start: func(c *Client, h *found) error { return c.DiscoverCharacteristics(5, 9, h) },
			rsp: "09 07 0400 02 0500 192a", want: ErrInvalidPDU},
		{name: "descriptor format 3", start: func(c *Client, h *found) error { return c.DiscoverDescriptors(1, 9, h) }, rsp: "05 03 0100 0229", want: ErrInvalidPDU},
		{name: "empty list", start: func(c *Client, h *found) error { return c.DiscoverServices(h) }, rsp: "11 06", want: ErrInvalidPDU},
		{name: "error for other request", start: func(c *Client, h *found) error { return c.DiscoverServices(h) }, rsp: "01 0a 0100 0a", want: ErrInvalidPDU},
		{name: "not found ends discovery", start: func(c *Client, h *found) error { return c.DiscoverServices(h) }, rsp: "01 10 0100 0a", want: nil},
		{name: "not found on read", start: func(c *Client, h *found) error { return c.Read(3, h) }, rsp: "01 0a 0300 0a", want: ErrResponse},
		{name: "write response with data", start: func(c *Client, h *found) error { return c.Write(3, []byte{1}) }, rsp: "13 00", want: ErrInvalidPDU},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLink(t, 23)
			if err := tt.start(l.c, l.got); err != nil {
				t.Fatal(err)
			}
			done, err := l.c.Handle(unhex(t, tt.rsp))
			if !done || err != tt.want || l.c.Busy() {
				t.Errorf("done %v, err %v, busy %v, want err %v", done, err, l.c.Busy(), tt.want)
			}
		})
	}
}

func TestClientStates(t *testing.T) {
	l := newLink(t, 23)
	if err := l.c.Write(3, make([]byte, 21)); err != ErrValueSize || l.c.Busy() {
		t.Errorf("value longer than MTU-3: %v, busy %v", err, l.c.Busy())
	}
	if done, err := l.c.Handle(unhex(t, "13")); done || err != ErrInvalidPDU {
		t.Errorf("response with no request: done %v, err %v", done, err)
	}

	// An indication is given to the handler and confirmed, also during a procedure.
	l.c.DiscoverServices(l.got)
	l.out.got = nil
	if done, err := l.c.Handle(unhex(t, "1d 0400 6869")); done || err != nil || l.got.notes[0] != "i4:hi" {
		t.Errorf("indication: done %v, err %v, %v", done, err, l.got.notes)
	}
	if len(l.out.got) != 1 || !bytes.Equal(l.out.got[0], []byte{opConfirm}) {
		t.Errorf("confirmation % x", l.out.got)
	}
	if err := l.c.Read(1, l.got); err != ErrBusy {
		t.Errorf("second request: %v", err)
	}

	// The timer starts at the first Tick after the request.
	if done, _ := l.c.Tick(1000); done {
		t.Error("timeout at the first Tick")
	}
	if done, _ := l.c.Tick(1000 + transactionMs - 1); done {
		t.Error("timeout too early")
	}
	if done, err := l.c.Tick(1000 + transactionMs); !done || err != ErrTimeout {
		t.Errorf("timeout: done %v, err %v", done, err)
	}
	if err := l.c.Read(1, l.got); err != ErrTimeout {
		t.Errorf("request after timeout: %v", err)
	}
	l.c.Close()
	if _, err := l.c.Handle(unhex(t, "1b 0100")); err != ErrClosed {
		t.Errorf("Handle after Close: %v", err)
	}
	if _, err := NewClient(nil, nil, nil, 0); err != ErrNilDependency {
		t.Errorf("nil dependencies: %v", err)
	}
	if ForClient([]byte{opWriteCmd}) || ForClient([]byte{opConfirm}) || !ForClient([]byte{opNotify}) || ForClient(nil) {
		t.Error("ForClient routes wrong")
	}
}
