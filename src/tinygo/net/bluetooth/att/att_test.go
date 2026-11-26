package att

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"runtime/memory/arena"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Custom 128-bit UUIDs in wire order.
var (
	customService = UUID{0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, 0x78, 0x56, 0x34, 0x12, 0x78, 0x56, 0x34, 0x12}
	customChar    = UUID{0xf1, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, 0x78, 0x56, 0x34, 0x12, 0x78, 0x56, 0x34, 0x12}
)

type battery struct{ level byte }

func (b *battery) ReadValue(_, offset uint16, dst []byte) (int, uint8) {
	if offset > 0 {
		return 0, CodeInvalidOffset
	}
	dst[0] = b.level
	return 1, 0
}
func (b *battery) WriteValue(uint16, []byte) uint8 { return CodeWriteNotPermitted }

type store struct{ value []byte }

func (s *store) ReadValue(_, offset uint16, dst []byte) (int, uint8) {
	return copyFrom(s.value, offset, dst)
}
func (s *store) WriteValue(_ uint16, v []byte) uint8 {
	s.value = append(s.value[:0], v...)
	return 0
}

type client struct{ got [][]byte }

func (c *client) SendATT(pdu []byte) error {
	c.got = append(c.got, append([]byte(nil), pdu...))
	return nil
}

type fixture struct {
	mem   *arena.Heap
	db    *DB
	srv   *Server
	cl    *client
	bat   *battery
	store *store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	mem, err := arena.NewHeap(make([]byte, 2048))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{mem: mem, cl: &client{}, bat: &battery{level: 90}, store: &store{}}
	table := make([]Attribute, 16)
	db, _ := NewDB(mem, table)
	must := func(_ uint16, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.AddService(UUID16(0x1800)))
	must(db.AddCharacteristic(UUID16(0x2a00), PropRead, PermRead, []byte("tiny"), nil))
	must(db.AddService(UUID16(0x180f)))
	must(db.AddCharacteristic(UUID16(0x2a19), PropRead|PropNotify, PermRead, nil, f.bat))
	must(db.AddCCCD())
	must(db.AddService(customService))
	must(db.AddCharacteristic(customChar, PropRead|PropWrite, PermRead|PermWrite, nil, f.store))
	f.db = db
	if f.srv, err = NewServer(mem, db, f.cl, 247); err != nil {
		t.Fatal(err)
	}
	return f
}

// exchange sends one request and returns the response, or nil when there is none.
func (f *fixture) exchange(t *testing.T, req string) []byte {
	t.Helper()
	f.cl.got = nil
	if err := f.srv.Handle(unhex(t, req)); err != nil {
		t.Fatal(err)
	}
	if len(f.cl.got) == 0 {
		return nil
	}
	return f.cl.got[0]
}

func TestDatabaseLayout(t *testing.T) {
	f := newFixture(t)
	if f.db.Len() != 10 {
		t.Errorf("database has %d attributes, want 10", f.db.Len())
	}
	if s, ok := UUID16(0x2a19).Short(); !ok || s != 0x2a19 {
		t.Error("UUID16 does not round trip")
	}
	if _, ok := customChar.Short(); ok || customChar.Len() != 16 {
		t.Error("a custom UUID looks like a 16-bit UUID")
	}
}

func TestDiscoveryAndReads(t *testing.T) {
	f := newFixture(t)
	custom := hex.EncodeToString(customService[:])
	customC := hex.EncodeToString(customChar[:])
	tests := []struct {
		name string
		req  string
		want string
	}{
		{name: "exchange MTU", req: "02 f700", want: "03 f700"},
		{name: "primary services, 16-bit", req: "10 0100 ffff 0028", want: "11 06 0100 0300 0018 0400 0700 0f18"},
		{name: "primary services, 128-bit", req: "10 0800 ffff 0028", want: "11 14 0800 0a00 " + custom},
		{name: "primary services, none left", req: "10 0b00 ffff 0028", want: "01 10 0b00 0a"},
		{name: "unsupported group type", req: "10 0100 ffff 0328", want: "01 10 0100 10"},
		{name: "service by UUID", req: "06 0100 ffff 0028 0f18", want: "07 0400 0700"},
		{name: "characteristics of battery service", req: "08 0400 0700 0328", want: "09 07 0500 12 0600 192a"},
		{name: "characteristic with 128-bit UUID", req: "08 0800 0a00 0328", want: "09 15 0900 0a 0a00 " + customC},
		{name: "descriptors", req: "04 0700 0700", want: "05 01 0700 0229"},
		{name: "two 16-bit types", req: "04 0800 0900", want: "05 01 0800 0028 0900 0328"},
		{name: "128-bit type", req: "04 0a00 0a00", want: "05 02 0a00 " + customC},
		{name: "read static value", req: "0a 0300", want: "0b 74696e79"},
		{name: "read handler value", req: "0a 0600", want: "0b 5a"},
		{name: "read blob", req: "0c 0300 0200", want: "0d 6e79"},
		{name: "read blob past the end", req: "0c 0300 0500", want: "01 0c 0300 07"},
		{name: "read invalid handle", req: "0a 0b00", want: "01 0a 0b00 01"},
		{name: "reversed handle range", req: "04 0500 0400", want: "01 04 0500 01"},
		{name: "unsupported request", req: "0e 0300 0600", want: "01 0e 0000 06"},
		{name: "malformed read", req: "0a 03", want: "01 0a 0000 04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.exchange(t, tt.req)
			if want := unhex(t, tt.want); !bytes.Equal(got, want) {
				t.Errorf("response = %x\nwant       %x", got, want)
			}
		})
	}
	if f.srv.MTU() != 247 {
		t.Errorf("MTU = %d, want 247", f.srv.MTU())
	}
}

func TestWritesAndNotify(t *testing.T) {
	f := newFixture(t)
	if err := f.srv.Notify(6, []byte{0x55}); err != ErrNotifyOff {
		t.Fatalf("Notify before the CCCD write error = %v, want ErrNotifyOff", err)
	}
	if got := f.exchange(t, "12 0700 0100"); !bytes.Equal(got, []byte{0x13}) {
		t.Fatalf("CCCD write response = %x", got)
	}
	if got := f.exchange(t, "0a 0700"); !bytes.Equal(got, unhex(t, "0b 0100")) {
		t.Errorf("CCCD read = %x", got)
	}
	f.cl.got = nil
	if err := f.srv.Notify(6, []byte{0x55}); err != nil {
		t.Fatal(err)
	}
	if len(f.cl.got) != 1 || !bytes.Equal(f.cl.got[0], unhex(t, "1b 0600 55")) {
		t.Errorf("notification = %x", f.cl.got)
	}
	if err := f.srv.Notify(3, []byte{1}); err != ErrHandle {
		t.Errorf("Notify on a value without a CCCD error = %v, want ErrHandle", err)
	}
	if err := f.srv.Notify(6, make([]byte, 21)); err != ErrMTU {
		t.Errorf("Notify larger than MTU-3 error = %v, want ErrMTU", err)
	}

	tests := []struct {
		name string
		req  string
		want string
	}{
		{name: "write read-only value", req: "12 0300 41", want: "01 12 0300 03"},
		{name: "write custom value", req: "12 0a00 68656c6c6f", want: "13"},
		{name: "CCCD with a wrong length", req: "12 0700 01", want: "01 12 0700 0d"},
		{name: "write invalid handle", req: "12 0b00 00", want: "01 12 0b00 01"},
		{name: "write command has no response", req: "52 0a00 776f726c64", want: ""},
		{name: "unknown command is ignored", req: "7f 0000", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.exchange(t, tt.req)
			if want := unhex(t, tt.want); !bytes.Equal(got, want) {
				t.Errorf("response = %x, want %x", got, want)
			}
		})
	}
	if string(f.store.value) != "world" {
		t.Errorf("custom value = %q, want %q", f.store.value, "world")
	}
}

func TestSmallMTUAndMemory(t *testing.T) {
	f := newFixture(t)
	// With the default MTU of 23, one Find Information response holds at most 5 16-bit entries.
	got := f.exchange(t, "04 0100 ffff")
	if got[0] != 0x05 || got[1] != 1 || len(got) != 2+5*4 {
		t.Errorf("Find Information at MTU 23 = %x", got)
	}
	held := f.mem.Stats().InUse
	for i := 0; i < 50; i++ {
		f.exchange(t, "0a 0600")
	}
	if f.mem.Stats().InUse != held {
		t.Errorf("requests changed memory in use from %d to %d", held, f.mem.Stats().InUse)
	}
	f.srv.Close()
	f.db.Close()
	if n := f.mem.Stats().InUse; n != 0 {
		t.Errorf("memory in use after Close = %d, want 0", n)
	}
	if err := f.srv.Handle([]byte{0x0a, 3, 0}); err != ErrClosed {
		t.Errorf("Handle after Close error = %v, want ErrClosed", err)
	}
}

func TestBuilderErrors(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 512))
	if _, err := NewDB(nil, make([]Attribute, 1)); err != ErrNilDependency {
		t.Errorf("nil memory error = %v", err)
	}
	db, _ := NewDB(mem, make([]Attribute, 3))
	if _, err := db.AddCharacteristic(UUID16(1), PropRead, PermRead, nil, nil); err != ErrOrder {
		t.Errorf("characteristic before service error = %v, want ErrOrder", err)
	}
	db.AddService(UUID16(0x180f))
	if _, err := db.AddCCCD(); err != ErrOrder {
		t.Errorf("CCCD without characteristic error = %v, want ErrOrder", err)
	}
	db.AddCharacteristic(UUID16(0x2a19), PropRead, PermRead, nil, nil)
	if _, err := db.AddService(UUID16(0x1800)); err != ErrFull {
		t.Errorf("full table error = %v, want ErrFull", err)
	}
	db.Close()
	if n := mem.Stats().InUse; n != 0 {
		t.Errorf("memory in use after Close = %d, want 0", n)
	}
}
