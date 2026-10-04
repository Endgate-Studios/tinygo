package peripheral

import (
	"math/rand"
	"testing"

	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

// protectedHandle is the value handle of a characteristic that needs an encrypted link. The
// battery service uses handles 1 to 4, so this service starts at 5.
const protectedHandle = 7

func protectedDB(db *att.DB) {
	db.AddService(att.UUID16(0x1234))
	db.AddCharacteristic(att.UUID16(0x5678), att.PropRead, att.PermRead|att.PermReadEncrypted, []byte{42}, nil)
}

// readProtected reads the protected value and returns the response.
func readProtected(h *harness) []byte {
	h.t.Helper()
	h.send(l2cap.CIDATT, []byte{0x0a, protectedHandle, 0x00})
	return h.pop(l2cap.CIDATT, h.frames[0].data[0])
}

func TestEncryptionPermission(t *testing.T) {
	h := newHarnessDB(t, &store{}, nil, protectedDB)
	h.start()
	peer := smp.Address{Type: 1, Addr: [6]byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x76}}
	h.connect(peer)

	// No bond yet: the server asks the peer to pair, which makes iOS show its pairing popup.
	if rsp := readProtected(h); rsp[0] != 0x01 || rsp[4] != att.CodeInsufficientAuth {
		t.Fatalf("read before pairing: % x, want Insufficient Authentication", rsp)
	}

	h.pair(peer)
	h.ltkRequest()
	h.encrypted()
	h.pop(l2cap.CIDSMP, 0x08)
	h.pop(l2cap.CIDSMP, 0x09)
	h.send(l2cap.CIDSMP, cat([]byte{0x08}, centralIRK[:]))
	h.send(l2cap.CIDSMP, cat([]byte{0x09, centralID.Type}, centralID.Addr[:]))
	if rsp := readProtected(h); rsp[0] != 0x0b || rsp[1] != 42 {
		t.Fatalf("read after pairing: % x, want the value 42", rsp)
	}

	// With a bond, the server asks the peer to encrypt, and it does not pair again.
	h.event(hci.EvtDisconnectionComplete, []byte{0, byte(conn), byte(conn >> 8), 0x13})
	h.command(hci.OpLESetAdvEnable)
	var rpa [6]byte
	if err := smp.NewRPA(h.cmem, &centralIRK, rand.New(rand.NewSource(12)), &rpa); err != nil {
		t.Fatal(err)
	}
	h.connect(smp.Address{Type: 1, Addr: rpa})
	if rsp := readProtected(h); rsp[0] != 0x01 || rsp[4] != att.CodeInsufficientEncrypt {
		t.Fatalf("read after reconnection: % x, want Insufficient Encryption", rsp)
	}
	h.ltkRequest()
	h.encrypted()
	if rsp := readProtected(h); rsp[0] != 0x0b || rsp[1] != 42 {
		t.Errorf("read after encryption with the bond: % x, want the value 42", rsp)
	}
}
