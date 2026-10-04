package att

import (
	"testing"

	"runtime/memory/arena"
)

// TestSecurityWriteNotifyCCCD covers the write path, the CCCD of a protected characteristic, and Notify.
func TestSecurityWriteNotifyCCCD(t *testing.T) {
	mem, _ := arena.NewHeap(make([]byte, 1024))
	db, _ := NewDB(mem, make([]Attribute, 4))
	db.AddService(UUID16(0x1234))
	st := &store{}
	value, _ := db.AddCharacteristic(UUID16(0x5678), PropWrite|PropNotify, PermRead|PermWrite|PermReadEncrypted|PermWriteEncrypted, nil, st)
	cccd, _ := db.AddCCCD()
	cl := &client{}
	srv, _ := NewServer(mem, db, cl, 64)

	write := func(h uint16, v ...byte) []byte {
		cl.got = nil
		srv.Handle(append([]byte{0x12, byte(h), byte(h >> 8)}, v...))
		return cl.got[0]
	}
	if rsp := write(value, 1); rsp[0] != 0x01 || rsp[4] != CodeInsufficientAuth {
		t.Errorf("write with no bond: % x", rsp)
	}
	srv.SetSecurity(false, true)
	if rsp := write(cccd, 1, 0); rsp[0] != 0x01 || rsp[4] != CodeInsufficientEncrypt {
		t.Errorf("CCCD write of a protected characteristic, bonded but not encrypted: % x", rsp)
	}
	srv.SetSecurity(true, true)
	if rsp := write(cccd, 1, 0); rsp[0] != 0x13 {
		t.Fatalf("CCCD write on an encrypted link: % x", rsp)
	}
	if err := srv.Notify(value, []byte{5}); err != nil {
		t.Errorf("Notify on an encrypted link: %v", err)
	}
	srv.SetSecurity(false, true)
	if err := srv.Notify(value, []byte{5}); err != ErrNotEncrypted {
		t.Errorf("Notify after encryption ended: %v", err)
	}
}
