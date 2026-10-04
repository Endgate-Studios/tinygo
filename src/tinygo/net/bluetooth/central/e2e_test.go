package central

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"

	"runtime/memory/arena"
	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/peripheral"
	"tinygo/net/bluetooth/smp"
)

const (
	centralHandle    = 0x0041
	peripheralHandle = 0x0040
)

var (
	centralAddr    = smp.Address{Type: 1, Addr: [6]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0xd1}}
	peripheralAddr = smp.Address{Type: 1, Addr: [6]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0xe2}}
	centralIRK     = [16]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	peripheralIRK  = [16]byte{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
)

type bondEntry struct {
	peer smp.Address
	bond smp.Bond
}

type bondStore struct{ entries []bondEntry }

func (s *bondStore) Bond(i int, peer *smp.Address, b *smp.Bond) bool {
	if i >= len(s.entries) {
		return false
	}
	*peer, *b = s.entries[i].peer, s.entries[i].bond
	return true
}

func (s *bondStore) SaveBond(peer *smp.Address, b *smp.Bond) error {
	s.entries = append(s.entries, bondEntry{*peer, *b})
	return nil
}

type delivery struct {
	toCentral bool
	pkt       []byte
}

// air is the simulated controller of both devices.
type air struct {
	t     *testing.T
	c     *Central
	p     *peripheral.Peripheral
	queue []delivery
	ltk   [16]byte // key of the pending LE Enable Encryption of the central
}

type centralSide struct{ a *air }
type peripheralSide struct{ a *air }

func (a *air) event(toCentral bool, code uint8, params []byte) {
	var b [260]byte
	n, _ := hci.PutEvent(b[:], code, params)
	a.queue = append(a.queue, delivery{toCentral, append([]byte(nil), b[:n]...)})
}

func (a *air) status(toCentral bool, op hci.Opcode, st uint8) {
	a.event(toCentral, hci.EvtCommandStatus, []byte{st, 1, byte(op), byte(op >> 8)})
}

func (a *air) complete(toCentral bool, op hci.Opcode) {
	a.event(toCentral, hci.EvtCommandComplete, []byte{1, byte(op), byte(op >> 8), 0})
}

func (a *air) encryptionChange(st, on uint8) {
	a.event(true, hci.EvtEncryptionChange, []byte{st, byte(centralHandle), byte(centralHandle >> 8), on})
	a.event(false, hci.EvtEncryptionChange, []byte{st, byte(peripheralHandle), byte(peripheralHandle >> 8), on})
}

// forward sends an ACL packet to the other device with its connection handle.
func (a *air) forward(b []byte, toCentral bool, handle uint16) error {
	acl, err := hci.ParseACL(b)
	if err != nil {
		return err
	}
	var out [300]byte
	n, err := hci.PutACL(out[:], handle, acl.PB, acl.Data)
	if err != nil {
		return err
	}
	a.queue = append(a.queue, delivery{toCentral, append([]byte(nil), out[:n]...)})
	return nil
}

func airConnComplete(role uint8, handle uint16, peer smp.Address) []byte {
	p := make([]byte, 19)
	p[0] = hci.SubConnectionComplete
	binary.LittleEndian.PutUint16(p[2:], handle)
	p[4], p[5] = role, peer.Type
	copy(p[6:12], peer.Addr[:])
	p[12], p[16] = 24, 200
	return p
}

func (s centralSide) WriteHCI(b []byte) error {
	a := s.a
	if b[0] == hci.TypeACL {
		return a.forward(b, false, peripheralHandle)
	}
	cmd, err := hci.ParseCommand(b)
	if err != nil {
		return err
	}
	switch cmd.Op {
	case hci.OpLECreateConnection:
		a.status(true, cmd.Op, 0)
		a.event(true, hci.EvtLEMeta, airConnComplete(0, centralHandle, peripheralAddr))
		a.event(false, hci.EvtLEMeta, airConnComplete(1, peripheralHandle, centralAddr))
	case hci.OpLEEnableEncryption:
		copy(a.ltk[:], cmd.Params[12:28])
		a.status(true, cmd.Op, 0)
		req := make([]byte, 13)
		req[0] = hci.SubLongTermKeyRequest
		binary.LittleEndian.PutUint16(req[1:], peripheralHandle)
		a.event(false, hci.EvtLEMeta, req)
	case hci.OpDisconnect:
		a.status(true, cmd.Op, 0)
		a.event(true, hci.EvtDisconnectionComplete, []byte{0, byte(centralHandle), byte(centralHandle >> 8), 0x16})
		a.event(false, hci.EvtDisconnectionComplete, []byte{0, byte(peripheralHandle), byte(peripheralHandle >> 8), 0x13})
	default:
		a.complete(true, cmd.Op)
	}
	return nil
}

func (s peripheralSide) WriteHCI(b []byte) error {
	a := s.a
	if b[0] == hci.TypeACL {
		return a.forward(b, true, centralHandle)
	}
	cmd, err := hci.ParseCommand(b)
	if err != nil {
		return err
	}
	a.complete(false, cmd.Op)
	switch cmd.Op {
	case hci.OpLELongTermKeyReply:
		// 0x06 is PIN or Key Missing (Core spec v5.4 Vol 1 Part F 2.6).
		if bytes.Equal(cmd.Params[2:18], a.ltk[:]) {
			a.encryptionChange(0, 1)
		} else {
			a.encryptionChange(0x06, 0)
		}
	case hci.OpLELongTermKeyNeg:
		a.encryptionChange(0x06, 0)
	}
	return nil
}

func (a *air) pump() {
	a.t.Helper()
	for n := 0; len(a.queue) > 0; n++ {
		if n > 5000 {
			a.t.Fatal("traffic does not stop")
		}
		d := a.queue[0]
		a.queue = a.queue[1:]
		var err error
		if d.toCentral {
			err = a.c.HandleHCI(d.pkt, 0)
		} else {
			err = a.p.HandleHCI(d.pkt, 0)
		}
		if err != nil {
			a.t.Fatalf("to central %v: % x: %v", d.toCentral, d.pkt, err)
		}
	}
}

type readValue struct{ got []byte }

func (r *readValue) ReadPart(_, _ uint16, data []byte) { r.got = append(r.got, data...) }

type bothBonds struct{ c, p *bondStore }

// newAir builds both devices. Both have identity keys and no IO, so they pair with Just Works.
func newAir(t *testing.T, bonds bothBonds) *air {
	t.Helper()
	a := &air{t: t}
	cmem, _ := arena.NewHeap(make([]byte, 8192))
	pmem, _ := arena.NewHeap(make([]byte, 8192))
	c, err := New(cmem, centralSide{a}, Config{
		MaxMTU: 64, MaxACL: 27, ACLBuffers: 1000,
		Own: centralAddr, RNG: rand.New(rand.NewSource(5)), Bonds: bonds.c, Bonding: true,
		IOCap: smp.IONoInputNoOutput, IRK: &centralIRK, Identity: centralAddr,
	})
	if err != nil {
		t.Fatal(err)
	}
	db, _ := att.NewDB(pmem, make([]att.Attribute, 8))
	db.AddService(att.UUID16(0x180f))
	db.AddCharacteristic(att.UUID16(0x2a19), att.PropRead, att.PermRead, []byte{93}, nil)
	p, err := peripheral.New(pmem, peripheralSide{a}, rand.New(rand.NewSource(6)), db, bonds.p, peripheral.Config{
		Own: peripheralAddr, Adv: hci.AdvParams{IntervalMin: 160, IntervalMax: 160, ChannelMap: hci.ChannelsAll},
		MaxMTU: 64, MaxACL: 27, ACLBuffers: 1000,
		IOCap: smp.IONoInputNoOutput, IRK: &peripheralIRK, Identity: peripheralAddr,
	})
	if err != nil {
		t.Fatal(err)
	}
	a.c, a.p = c, p
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	a.pump()
	if p.State() != peripheral.StateAdvertising {
		t.Fatalf("peripheral state %d", p.State())
	}
	return a
}

func (a *air) connect() {
	a.t.Helper()
	if err := a.c.Connect(&hci.ConnParams{ScanInterval: 0x60, ScanWindow: 0x30, PeerAddrType: peripheralAddr.Type, PeerAddr: peripheralAddr.Addr, OwnAddrType: 1, IntervalMin: 24, IntervalMax: 40, SupervisionTimeout: 200}); err != nil {
		a.t.Fatal(err)
	}
	a.pump()
	if a.c.ConnState() != ConnConnected || a.p.State() != peripheral.StateConnected {
		a.t.Fatalf("states %d and %d after connect", a.c.ConnState(), a.p.State())
	}
}

func (a *air) disconnect() {
	a.t.Helper()
	if err := a.c.Disconnect(0x13); err != nil {
		a.t.Fatal(err)
	}
	a.pump()
	if a.c.ConnState() != ConnIdle || a.p.State() != peripheral.StateAdvertising {
		a.t.Fatalf("states %d and %d after disconnect", a.c.ConnState(), a.p.State())
	}
}

func TestPairBondAndReconnect(t *testing.T) {
	bonds := bothBonds{&bondStore{}, &bondStore{}}
	a := newAir(t, bonds)
	a.connect()
	if err := a.c.Secure(); err != nil {
		t.Fatal(err)
	}
	a.pump()
	if !a.c.Encrypted() || !a.p.Encrypted() || a.c.Pairing() != smp.StatePaired || a.p.Pairing() != smp.StatePaired {
		t.Fatalf("encrypted %v and %v, pairing %d and %d", a.c.Encrypted(), a.p.Encrypted(), a.c.Pairing(), a.p.Pairing())
	}
	if len(bonds.c.entries) != 1 || len(bonds.p.entries) != 1 {
		t.Fatalf("bonds %d and %d", len(bonds.c.entries), len(bonds.p.entries))
	}
	cb, pb := bonds.c.entries[0].bond, bonds.p.entries[0].bond
	if cb.LTK != pb.LTK || cb.PeerIdentity != peripheralAddr || pb.PeerIdentity != centralAddr || cb.PeerIRK != peripheralIRK || pb.PeerIRK != centralIRK {
		t.Errorf("central bond %+v, peripheral bond %+v", cb, pb)
	}

	// The GATT client of the central reads the battery level over the encrypted link.
	r := &readValue{}
	if err := a.c.Client().Read(3, r); err != nil {
		t.Fatal(err)
	}
	a.pump()
	if !bytes.Equal(r.got, []byte{93}) {
		t.Errorf("read % x", r.got)
	}

	// After a reconnection, both sides encrypt with the stored bond and do not pair again.
	a.disconnect()
	a.connect()
	if err := a.c.Secure(); err != nil {
		t.Fatal(err)
	}
	a.pump()
	if !a.c.Encrypted() || !a.p.Encrypted() || a.c.Pairing() != smp.StateIdle || a.p.Pairing() != smp.StateIdle {
		t.Errorf("reconnection: encrypted %v and %v, pairing %d and %d", a.c.Encrypted(), a.p.Encrypted(), a.c.Pairing(), a.p.Pairing())
	}
	if len(bonds.c.entries) != 1 || len(bonds.p.entries) != 1 {
		t.Errorf("bonds %d and %d after reconnection", len(bonds.c.entries), len(bonds.p.entries))
	}
}

// When the peripheral lost its bond, the encryption attempt fails on both sides.
func TestReconnectWithLostBond(t *testing.T) {
	bonds := bothBonds{&bondStore{}, &bondStore{}}
	a := newAir(t, bonds)
	a.connect()
	a.c.Secure()
	a.pump()
	a.disconnect()
	bonds.p.entries = nil
	a.connect()
	if err := a.c.Secure(); err != nil {
		t.Fatal(err)
	}
	a.pump()
	if a.c.Encrypted() || a.p.Encrypted() {
		t.Errorf("encrypted %v and %v with a lost bond", a.c.Encrypted(), a.p.Encrypted())
	}
}

func TestSecureNeedsSetup(t *testing.T) {
	c, err := New(newMem(), &controller{}, Config{MaxACL: 27})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Secure(); err != ErrNotConnected {
		t.Errorf("Secure with no connection: %v", err)
	}
	c.conn = ConnConnected
	if err := c.Secure(); err != ErrNoPairing {
		t.Errorf("Secure with no pairing setup: %v", err)
	}
}
