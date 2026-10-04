package peripheral

import (
	"bytes"
	"crypto/aes"
	"crypto/ecdh"
	"encoding/binary"
	"math/rand"
	"testing"

	"runtime/memory/arena"
	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

const conn = 0x0040

type frame struct {
	cid  uint16
	data []byte
}

type bondEntry struct {
	peer smp.Address
	bond smp.Bond
}

type store struct{ entries []bondEntry }

func (s *store) Bond(i int, peer *smp.Address, b *smp.Bond) bool {
	if i >= len(s.entries) {
		return false
	}
	*peer, *b = s.entries[i].peer, s.entries[i].bond
	return true
}

func (s *store) SaveBond(peer *smp.Address, b *smp.Bond) error {
	s.entries = append(s.entries, bondEntry{*peer, *b})
	return nil
}

type results struct{ done []error }

func (r *results) ProcedureDone(err error) { r.done = append(r.done, err) }

// harness is the controller and the central. Packets from the peripheral collect in out, and the
// central reassembles ACL data into frames.
type harness struct {
	t      *testing.T
	p      *Peripheral
	mem    *arena.Heap
	db     *att.DB
	bonds  *store
	res    *results
	cmem   *arena.Heap
	rx     *l2cap.Reassembler
	out    [][]byte
	cmds   []hci.Command
	frames []frame
	now    uint32
}

func (h *harness) WriteHCI(b []byte) error {
	h.out = append(h.out, append([]byte(nil), b...))
	return nil
}

func (h *harness) HandleFrame(_, cid uint16, payload []byte) {
	h.frames = append(h.frames, frame{cid, append([]byte(nil), payload...)})
}

// WriteACL delivers ACL data from the central to the peripheral.
func (h *harness) WriteACL(pkt []byte) error { return h.p.HandleHCI(pkt, h.now) }

var (
	own        = smp.Address{Type: 1, Addr: [6]byte{1, 2, 3, 4, 5, 0xc6}}
	centralIRK = [16]byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f}
	centralID  = smp.Address{Type: 0, Addr: [6]byte{0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6}}
	ownIRK     = [16]byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
)

func newHarness(t *testing.T, bonds *store) *harness {
	t.Helper()
	return newHarnessWith(t, bonds, nil)
}

// newHarnessWith lets change change the Config before New, when it is not nil.
func newHarnessWith(t *testing.T, bonds *store, change func(*Config)) *harness {
	t.Helper()
	return newHarnessDB(t, bonds, change, nil)
}

// newHarnessDB also lets extend add attributes after the battery service, when it is not nil.
func newHarnessDB(t *testing.T, bonds *store, change func(*Config), extend func(*att.DB)) *harness {
	t.Helper()
	mem, _ := arena.NewHeap(make([]byte, 4096))
	cmem, _ := arena.NewHeap(make([]byte, 4096))
	h := &harness{t: t, mem: mem, cmem: cmem, bonds: bonds, res: &results{}}
	db, _ := att.NewDB(mem, make([]att.Attribute, 8))
	db.AddService(att.UUID16(0x180f))
	db.AddCharacteristic(att.UUID16(0x2a19), att.PropRead|att.PropNotify, att.PermRead, []byte{77}, nil)
	db.AddCCCD()
	if extend != nil {
		extend(db)
	}
	h.db = db
	cfg := Config{
		Own:        own,
		Adv:        hci.AdvParams{IntervalMin: 160, IntervalMax: 160, OwnAddrType: hci.AddrRandom, ChannelMap: hci.ChannelsAll},
		AdvData:    []byte{2, 1, 6},
		IOCap:      smp.IONoInputNoOutput,
		MaxMTU:     64,
		MaxACL:     27,
		ACLBuffers: 1000,
		IRK:        &ownIRK,
		Identity:   own,
		Done:       h.res,
	}
	if change != nil {
		change(&cfg)
	}
	p, err := New(mem, h, rand.New(rand.NewSource(3)), db, bonds, cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.p = p
	h.rx, _ = l2cap.NewReassembler(cmem, h, 512)
	return h
}

// pump moves the packets of the peripheral to the controller and the central.
func (h *harness) pump() {
	h.t.Helper()
	for _, b := range h.out {
		switch b[0] {
		case hci.TypeCommand:
			c, err := hci.ParseCommand(b)
			if err != nil {
				h.t.Fatal(err)
			}
			h.cmds = append(h.cmds, c)
		case hci.TypeACL:
			a, err := hci.ParseACL(b)
			if err != nil || a.Handle != conn {
				h.t.Fatalf("ACL % x: %v", b, err)
			}
			if err := h.rx.HandleACL(a); err != nil {
				h.t.Fatal(err)
			}
		}
	}
	h.out = nil
}

func (h *harness) event(code uint8, params []byte) {
	h.t.Helper()
	var b [260]byte
	n, _ := hci.PutEvent(b[:], code, params)
	if err := h.p.HandleHCI(b[:n], h.now); err != nil {
		h.t.Fatalf("event %#x: %v", code, err)
	}
	h.pump()
}

func (h *harness) meta(sub uint8, params []byte) {
	h.event(hci.EvtLEMeta, append([]byte{sub}, params...))
}

// command pops the next command, checks its opcode, and answers with Command Complete and status 0.
func (h *harness) command(op hci.Opcode) hci.Command {
	h.t.Helper()
	if len(h.cmds) == 0 {
		h.t.Fatalf("no command, want %#04x", uint16(op))
	}
	c := h.cmds[0]
	h.cmds = h.cmds[1:]
	if c.Op != op {
		h.t.Fatalf("command %#04x, want %#04x", uint16(c.Op), uint16(op))
	}
	var p [4]byte
	p[0] = 1
	binary.LittleEndian.PutUint16(p[1:], uint16(op))
	h.event(hci.EvtCommandComplete, p[:])
	return c
}

func (h *harness) send(cid uint16, payload []byte) {
	h.t.Helper()
	if err := l2cap.Send(h, h.cmem, conn, cid, payload, 27); err != nil {
		h.t.Fatal(err)
	}
	h.pump()
}

func (h *harness) pop(cid uint16, code byte) []byte {
	h.t.Helper()
	if len(h.frames) == 0 {
		h.t.Fatalf("no frame, want channel %#x code %#x", cid, code)
	}
	f := h.frames[0]
	h.frames = h.frames[1:]
	if f.cid != cid || f.data[0] != code {
		h.t.Fatalf("frame on %#x: % x, want channel %#x code %#x", f.cid, f.data, cid, code)
	}
	return f.data
}

func (h *harness) start() {
	h.t.Helper()
	if err := h.p.Start(); err != nil {
		h.t.Fatal(err)
	}
	h.pump()
	h.command(hci.OpReset)
	h.command(hci.OpLESetAdvParams)
	if c := h.command(hci.OpLESetAdvData); c.Params[0] != 3 {
		h.t.Errorf("advertising data % x", c.Params)
	}
	h.command(hci.OpLESetAdvEnable)
	if h.p.State() != StateAdvertising {
		h.t.Fatalf("state %d after setup", h.p.State())
	}
}

func (h *harness) connect(peer smp.Address) {
	h.t.Helper()
	p := make([]byte, 18)
	binary.LittleEndian.PutUint16(p[1:], conn)
	p[3], p[4] = 1, peer.Type
	copy(p[5:11], peer.Addr[:])
	h.meta(hci.SubConnectionComplete, p)
	if h.p.State() != StateConnected {
		h.t.Fatalf("state %d after connection", h.p.State())
	}
}

// ltkRequest sends an LTK request with EDIV and Rand zero and returns the reply command.
func (h *harness) ltkRequest() hci.Command {
	h.t.Helper()
	p := make([]byte, 12)
	binary.LittleEndian.PutUint16(p, conn)
	h.meta(hci.SubLongTermKeyRequest, p)
	if len(h.cmds) == 0 {
		h.t.Fatal("no reply to the LTK request")
	}
	c := h.cmds[0]
	h.command(c.Op)
	return c
}

func (h *harness) encrypted() {
	h.t.Helper()
	var p [4]byte
	binary.LittleEndian.PutUint16(p[1:], conn)
	p[3] = 1
	h.event(hci.EvtEncryptionChange, p[:])
}

func cmac(key, msg []byte) []byte {
	c, _ := aes.NewCipher(key)
	var l, k1, k2 [16]byte
	c.Encrypt(l[:], l[:])
	dbl := func(dst, src []byte) {
		var carry byte
		for i := 15; i >= 0; i-- {
			dst[i], carry = src[i]<<1|carry, src[i]>>7
		}
		if src[0]&0x80 != 0 {
			dst[15] ^= 0x87
		}
	}
	dbl(k1[:], l[:])
	dbl(k2[:], k1[:])
	n := max((len(msg)+15)/16, 1)
	last := make([]byte, 16)
	if len(msg) > 0 && len(msg)%16 == 0 {
		copy(last, msg[(n-1)*16:])
		for i := range last {
			last[i] ^= k1[i]
		}
	} else {
		rest := msg[(n-1)*16:]
		copy(last, rest)
		last[len(rest)] = 0x80
		for i := range last {
			last[i] ^= k2[i]
		}
	}
	x := make([]byte, 16)
	for i := 0; i < n-1; i++ {
		for j := range x {
			x[j] ^= msg[i*16+j]
		}
		c.Encrypt(x, x)
	}
	for j := range x {
		x[j] ^= last[j]
	}
	c.Encrypt(x, x)
	return x
}

func rev(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func spec(a smp.Address) []byte { return append([]byte{a.Type}, rev(a.Addr[:])...) }

// pair runs LE Secure Connections Just Works from the central with bonding and identity keys in
// both directions, and returns the LTK in wire order.
func (h *harness) pair(peer smp.Address) []byte {
	t := h.t
	rng := rand.New(rand.NewSource(9))
	preq := []byte{0x01, smp.IONoInputNoOutput, 0, 0x09, 16, 0x02, 0x02}
	h.send(l2cap.CIDSMP, preq)
	pres := h.pop(l2cap.CIDSMP, 0x02)
	ka, _ := ecdhKey(rng)
	pka := ka.PublicKey().Bytes()
	h.send(l2cap.CIDSMP, cat([]byte{0x0c}, rev(pka[1:33]), rev(pka[33:])))
	pk := h.pop(l2cap.CIDSMP, 0x0c)
	cb := rev(h.pop(l2cap.CIDSMP, 0x03)[1:])
	pkbx, pkby := rev(pk[1:33]), rev(pk[33:])
	na := make([]byte, 16)
	rng.Read(na)
	h.send(l2cap.CIDSMP, cat([]byte{0x04}, rev(na)))
	nb := rev(h.pop(l2cap.CIDSMP, 0x04)[1:])
	if !bytes.Equal(cb, cmac(nb, cat(pkbx, pka[1:33], []byte{0}))) {
		t.Fatal("Cb does not match")
	}
	pub, _ := ecdh.P256().NewPublicKey(cat([]byte{4}, pkbx, pkby))
	dh, _ := ka.ECDH(pub)
	tk := cmac([]byte{0x6c, 0x88, 0x83, 0x91, 0xaa, 0xf5, 0xa5, 0x38, 0x60, 0x37, 0x0b, 0xdb, 0x5a, 0x60, 0x83, 0xbe}, dh)
	a, b := spec(peer), spec(own)
	keyID := []byte{0x62, 0x74, 0x6c, 0x65}
	macKey := cmac(tk, cat([]byte{0}, keyID, na, nb, a, b, []byte{1, 0}))
	ltk := cmac(tk, cat([]byte{1}, keyID, na, nb, a, b, []byte{1, 0}))
	r := make([]byte, 16)
	ea := cmac(macKey, cat(na, nb, r, []byte{preq[3], preq[2], preq[1]}, a, b))
	h.send(l2cap.CIDSMP, cat([]byte{0x0d}, rev(ea)))
	eb := rev(h.pop(l2cap.CIDSMP, 0x0d)[1:])
	if !bytes.Equal(eb, cmac(macKey, cat(nb, na, r, []byte{pres[3], pres[2], pres[1]}, b, a))) {
		t.Fatal("Eb does not match")
	}
	return rev(ltk)
}

func TestPairBondReconnect(t *testing.T) {
	bonds := &store{}
	h := newHarness(t, bonds)
	h.start()
	peer := smp.Address{Type: 1, Addr: [6]byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x76}} // private address
	h.connect(peer)

	// GATT server: the central discovers services and exchanges the MTU.
	h.send(l2cap.CIDATT, []byte{0x10, 0x01, 0x00, 0xff, 0xff, 0x00, 0x28})
	if rsp := h.pop(l2cap.CIDATT, 0x11); !bytes.Equal(rsp, []byte{0x11, 0x06, 0x01, 0x00, 0x04, 0x00, 0x0f, 0x18}) {
		t.Errorf("service response % x", rsp)
	}
	h.send(l2cap.CIDATT, []byte{0x02, 0xf7, 0x00})
	h.pop(l2cap.CIDATT, 0x03)
	if h.p.Client().MTU() != 64 || h.p.Server().MTU() != 64 {
		t.Errorf("MTU client %d, server %d", h.p.Client().MTU(), h.p.Server().MTU())
	}

	// GATT client: the peripheral discovers the services of the central, which has none.
	if err := h.p.Client().DiscoverServices(nil); err != nil {
		t.Fatal(err)
	}
	h.pump()
	h.pop(l2cap.CIDATT, 0x10)
	h.send(l2cap.CIDATT, []byte{0x01, 0x10, 0x01, 0x00, 0x0a})
	if len(h.res.done) != 1 || h.res.done[0] != nil {
		t.Errorf("procedure results %v", h.res.done)
	}

	// Signaling: a request that the peripheral does not know gets Command Reject.
	h.send(l2cap.CIDSignaling, []byte{0x14, 0x07, 0x00, 0x00})
	if rej := h.pop(l2cap.CIDSignaling, l2cap.SigCommandReject); rej[1] != 0x07 {
		t.Errorf("reject % x", rej)
	}

	ltk := h.pair(peer)
	if c := h.ltkRequest(); c.Op != hci.OpLELongTermKeyReply || !bytes.Equal(c.Params[2:], ltk) {
		t.Fatalf("LTK reply %#04x % x, want % x", uint16(c.Op), c.Params, ltk)
	}
	h.encrypted()
	if info := h.pop(l2cap.CIDSMP, 0x08); !bytes.Equal(info[1:], ownIRK[:]) {
		t.Errorf("identity information % x", info)
	}
	h.pop(l2cap.CIDSMP, 0x09)
	if len(bonds.entries) != 0 {
		t.Fatal("bond saved before key distribution ended")
	}
	h.send(l2cap.CIDSMP, cat([]byte{0x08}, centralIRK[:]))
	h.send(l2cap.CIDSMP, cat([]byte{0x09, centralID.Type}, centralID.Addr[:]))
	if len(bonds.entries) != 1 || h.p.Pairing() != smp.StatePaired || !h.p.Encrypted() {
		t.Fatalf("bonds %d, pairing state %d", len(bonds.entries), h.p.Pairing())
	}
	saved := bonds.entries[0]
	if saved.peer != peer || !bytes.Equal(saved.bond.LTK[:], ltk) || saved.bond.PeerIRK != centralIRK || saved.bond.PeerIdentity != centralID || !saved.bond.Bonded {
		t.Errorf("saved bond %+v", saved)
	}

	// Disconnection: advertising starts again.
	h.event(hci.EvtDisconnectionComplete, []byte{0, byte(conn), byte(conn >> 8), 0x13})
	h.command(hci.OpLESetAdvEnable)
	if h.p.State() != StateAdvertising || h.p.Encrypted() {
		t.Fatalf("state %d after disconnection", h.p.State())
	}

	// Reconnection from a new resolvable private address of the central.
	var rpa [6]byte
	if err := smp.NewRPA(h.cmem, &centralIRK, rand.New(rand.NewSource(4)), &rpa); err != nil {
		t.Fatal(err)
	}
	h.connect(smp.Address{Type: 1, Addr: rpa})
	if c := h.ltkRequest(); c.Op != hci.OpLELongTermKeyReply || !bytes.Equal(c.Params[2:], ltk) {
		t.Errorf("LTK reply after reconnection %#04x % x", uint16(c.Op), c.Params)
	}
	h.encrypted()
	if len(h.frames) != 0 || len(bonds.entries) != 1 {
		t.Errorf("frames % x, bonds %d after encryption with a stored bond", h.frames, len(bonds.entries))
	}
	h.event(hci.EvtDisconnectionComplete, []byte{0, byte(conn), byte(conn >> 8), 0x13})
	h.command(hci.OpLESetAdvEnable)

	// A peer with no bond gets a negative reply.
	h.connect(smp.Address{Type: 1, Addr: [6]byte{1, 1, 1, 1, 1, 0x41}})
	if c := h.ltkRequest(); c.Op != hci.OpLELongTermKeyNeg {
		t.Errorf("unknown peer: command %#04x", uint16(c.Op))
	}

	h.p.Close()
	h.db.Close()
	h.rx.Close()
	if n := h.mem.Stats().InUse; n != 0 {
		t.Errorf("peripheral memory in use %d after Close", n)
	}
}

func TestSetupFailureAndCredits(t *testing.T) {
	h := newHarness(t, &store{})
	h.p.Start()
	h.pump()
	h.command(hci.OpReset)
	// The controller rejects the advertising parameters with status 0x12, invalid parameters.
	var b [8]byte
	n, _ := hci.PutEvent(b[:], hci.EvtCommandComplete, []byte{1, 0x06, 0x20, 0x12})
	if err := h.p.HandleHCI(b[:n], 0); err != ErrController || h.p.State() != StateFailed || h.p.Status() != 0x12 {
		t.Errorf("err %v, state %d, status %#x", err, h.p.State(), h.p.Status())
	}

	// A second command before the controller gives a credit back.
	h = newHarness(t, &store{})
	h.p.Start()
	if err := h.p.Start(); err != ErrNoCredits {
		t.Errorf("command with no credit: %v", err)
	}

	if _, err := New(nil, nil, nil, nil, nil, Config{}); err != ErrNilDependency {
		t.Errorf("nil dependencies: %v", err)
	}
	mem, _ := arena.NewHeap(make([]byte, 1024))
	db, _ := att.NewDB(mem, make([]att.Attribute, 2))
	if _, err := New(mem, h, rand.New(rand.NewSource(1)), db, &store{}, Config{MaxACL: 26}); err != ErrConfig {
		t.Errorf("small MaxACL: %v", err)
	}
}

func TestACLCredits(t *testing.T) {
	h := newHarness(t, &store{})
	h.p.cfg.ACLBuffers = 1
	h.start()
	h.connect(smp.Address{Addr: [6]byte{1, 2, 3, 4, 5, 6}})
	// The MTU response takes the only ACL buffer.
	h.send(l2cap.CIDATT, []byte{0x02, 0x40, 0x00})
	h.pop(l2cap.CIDATT, 0x03)
	read := []byte{0x0a, 0x03, 0x00}
	if err := l2cap.Send(h, h.cmem, conn, l2cap.CIDATT, read, 27); err != ErrNoCredits {
		t.Fatalf("read with no ACL buffer: %v", err)
	}
	// Number Of Completed Packets gives the buffer back.
	h.event(hci.EvtNumCompletedPackets, []byte{1, byte(conn), byte(conn >> 8), 1, 0})
	h.send(l2cap.CIDATT, read)
	if rsp := h.pop(l2cap.CIDATT, 0x0b); rsp[1] != 77 {
		t.Errorf("read after a credit came back % x", rsp)
	}
}
