package eapol

import (
	"bytes"
	stdaes "crypto/aes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"testing"
	"unsafe"

	"runtime/memory/arena"
	"tinygo/net/wifi/ieee80211"
)

// checkedMemory is an arena heap that records whether each freed block was zeroed first.
type checkedMemory struct {
	*arena.Heap
	sizes      map[unsafe.Pointer]uintptr
	dirtyFrees int
}

func newCheckedMemory(t *testing.T) *checkedMemory {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 8<<10))
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

// counterRandom returns 1, 2, 3, ... so nonces are fixed in tests.
type counterRandom struct{ n byte }

func (r *counterRandom) Read(b []byte) (int, error) {
	for i := range b {
		r.n++
		b[i] = r.n
	}
	return len(b), nil
}

type failingRandom struct{}

func (failingRandom) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// authenticator is an independent access point side built only on the standard library.
type authenticator struct {
	t       *testing.T
	pmk     []byte
	aa, spa ieee80211.MAC
	anonce  [32]byte
	snonce  [32]byte
	ptk     []byte
	replay  uint64
	rsn     []byte
	gtk     []byte
	gtkID   byte
	rsc     [8]byte
}

var (
	apAddr  = ieee80211.MAC{0x02, 0, 0, 0, 0, 0xaa}
	staAddr = ieee80211.MAC{0x02, 0, 0, 0, 0, 0x01}
	rsnElem = []byte{48, 20, 1, 0, 0, 0x0f, 0xac, 4, 1, 0, 0, 0x0f, 0xac, 4, 1, 0, 0, 0x0f, 0xac, 2, 0x0c, 0}
)

func pbkdf2(pass, salt []byte, iter, n int) []byte {
	var out []byte
	for block := uint32(1); len(out) < n; block++ {
		mac := hmac.New(sha1.New, pass)
		mac.Write(salt)
		binary.Write(mac, binary.BigEndian, block)
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha1.New, pass)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:n]
}

func refPRF(key []byte, label string, data []byte, n int) []byte {
	var out []byte
	for i := byte(0); len(out) < n; i++ {
		h := hmac.New(sha1.New, key)
		h.Write([]byte(label))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{i})
		out = h.Sum(out)
	}
	return out[:n]
}

func refWrap(kek, key []byte) []byte {
	b, _ := stdaes.NewCipher(kek)
	n := len(key) / 8
	a := uint64(0xA6A6A6A6A6A6A6A6)
	r := append([]byte(nil), key...)
	var blk [16]byte
	for j := 0; j <= 5; j++ {
		for i := 1; i <= n; i++ {
			binary.BigEndian.PutUint64(blk[:8], a)
			copy(blk[8:], r[8*(i-1):8*i])
			b.Encrypt(blk[:], blk[:])
			a = binary.BigEndian.Uint64(blk[:8]) ^ uint64(n*j+i)
			copy(r[8*(i-1):8*i], blk[8:])
		}
	}
	out := make([]byte, 8, 8+len(r))
	binary.BigEndian.PutUint64(out, a)
	return append(out, r...)
}

func newAuthenticator(t *testing.T) *authenticator {
	a := &authenticator{t: t, pmk: pbkdf2([]byte("password"), []byte("IEEE"), 4096, 32), aa: apAddr, spa: staAddr, rsn: rsnElem}
	for i := range a.anonce {
		a.anonce[i] = byte(0x80 + i)
	}
	a.gtk = bytes.Repeat([]byte{0x6b}, 16)
	a.gtkID = 1
	return a
}

func (a *authenticator) frame(info uint16, nonce []byte, data []byte) []byte {
	a.replay++
	f := make([]byte, offData+len(data))
	f[0], f[1] = 2, typeKey
	binary.BigEndian.PutUint16(f[2:], uint16(len(f)-headerLen))
	f[headerLen] = descRSN
	binary.BigEndian.PutUint16(f[offKeyInfo:], info)
	binary.BigEndian.PutUint16(f[offKeyLen:], 16)
	binary.BigEndian.PutUint64(f[offReplay:], a.replay)
	copy(f[offNonce:], nonce)
	copy(f[offRSC:], a.rsc[:])
	binary.BigEndian.PutUint16(f[offDataLen:], uint16(len(data)))
	copy(f[offData:], data)
	return f
}

func (a *authenticator) sign(f []byte) {
	h := hmac.New(sha1.New, a.ptk[:16])
	h.Write(f)
	copy(f[offMIC:], h.Sum(nil)[:16])
}

func (a *authenticator) checkMIC(f []byte) bool {
	c := append([]byte(nil), f...)
	clear(c[offMIC : offMIC+16])
	h := hmac.New(sha1.New, a.ptk[:16])
	h.Write(c)
	return hmac.Equal(h.Sum(nil)[:16], f[offMIC:offMIC+16])
}

func (a *authenticator) message1() []byte {
	return a.frame(infoVersionAES|infoPairwise|infoAck, a.anonce[:], nil)
}

// readMessage2 checks message 2 and derives the PTK from its SNonce.
func (a *authenticator) readMessage2(f []byte) {
	a.t.Helper()
	info := binary.BigEndian.Uint16(f[offKeyInfo:])
	if info != infoVersionAES|infoPairwise|infoMIC {
		a.t.Fatalf("message 2 key info = %#x", info)
	}
	if binary.BigEndian.Uint64(f[offReplay:]) != a.replay {
		a.t.Fatal("message 2 replay counter does not echo message 1")
	}
	copy(a.snonce[:], f[offNonce:offNonce+32])
	data := f[offData : offData+int(binary.BigEndian.Uint16(f[offDataLen:]))]
	if !bytes.Equal(data, rsnElem) {
		a.t.Fatalf("message 2 key data = %x, want the RSN element", data)
	}
	// Min and Max of each pair (IEEE 802.11-2020 12.7.1.3).
	var d []byte
	d = append(d, sorted(a.aa[:], a.spa[:])...)
	d = append(d, sorted(a.anonce[:], a.snonce[:])...)
	a.ptk = refPRF(a.pmk, "Pairwise key expansion", d, 48)
	if !a.checkMIC(f) {
		a.t.Fatal("message 2 MIC is wrong")
	}
}

func sorted(x, y []byte) []byte {
	if bytes.Compare(x, y) > 0 {
		x, y = y, x
	}
	return append(append([]byte(nil), x...), y...)
}

func (a *authenticator) gtkKDE() []byte {
	kde := []byte{0xdd, byte(6 + len(a.gtk)), 0x00, 0x0f, 0xac, 1, a.gtkID, 0}
	return append(kde, a.gtk...)
}

func pad(b []byte) []byte {
	if len(b)%8 != 0 || len(b) < 16 {
		b = append(b, 0xdd)
		for len(b)%8 != 0 || len(b) < 16 {
			b = append(b, 0)
		}
	}
	return b
}

func (a *authenticator) message3(rsn []byte) []byte {
	data := refWrap(a.ptk[16:32], pad(append(append([]byte(nil), rsn...), a.gtkKDE()...)))
	f := a.frame(infoVersionAES|infoPairwise|infoInstall|infoAck|infoMIC|infoSecure|infoEncrypted, a.anonce[:], data)
	a.sign(f)
	return f
}

func (a *authenticator) group1() []byte {
	data := refWrap(a.ptk[16:32], pad(a.gtkKDE()))
	f := a.frame(infoVersionAES|infoAck|infoMIC|infoSecure|infoEncrypted, nil, data)
	a.sign(f)
	return f
}

func (a *authenticator) readReply(f []byte, wantInfo uint16) {
	a.t.Helper()
	if info := binary.BigEndian.Uint16(f[offKeyInfo:]); info != wantInfo {
		a.t.Fatalf("reply key info = %#x, want %#x", info, wantInfo)
	}
	if binary.BigEndian.Uint64(f[offReplay:]) != a.replay || !a.checkMIC(f) {
		a.t.Fatal("reply replay counter or MIC is wrong")
	}
}

func newSupplicant(t *testing.T, mem Memory) *Supplicant {
	t.Helper()
	s, err := New(mem, &counterRandom{}, []byte("password"), []byte("IEEE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(apAddr, staAddr, rsnElem, rsnElem); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFourWayAndGroupHandshake(t *testing.T) {
	mem := newCheckedMemory(t)
	s := newSupplicant(t, mem)
	a := newAuthenticator(t)
	reply := make([]byte, 256)

	n, ev, err := s.Handle(a.message1(), reply)
	if err != nil || ev != EventNone || n == 0 {
		t.Fatalf("message 1: n %d, event %d, %v", n, ev, err)
	}
	a.readMessage2(reply[:n])
	if s.TK() != nil {
		t.Error("TK is available before message 3")
	}

	a.rsc = [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	n, ev, err = s.Handle(a.message3(rsnElem), reply)
	if err != nil || ev != EventPairwise {
		t.Fatalf("message 3: event %d, %v", ev, err)
	}
	if !bytes.Equal(s.GTKRSC(), a.rsc[:]) {
		t.Errorf("GTK RSC = %x, want %x", s.GTKRSC(), a.rsc)
	}
	a.readReply(reply[:n], infoVersionAES|infoPairwise|infoMIC|infoSecure)
	if !bytes.Equal(s.TK(), a.ptk[32:48]) {
		t.Errorf("TK = %x, want %x", s.TK(), a.ptk[32:48])
	}
	if gtk, id := s.GTK(); !bytes.Equal(gtk, a.gtk) || id != 1 {
		t.Errorf("GTK = %x id %d, want %x id 1", gtk, id, a.gtk)
	}
	held := mem.Heap.Stats().InUse

	a.gtk, a.gtkID = bytes.Repeat([]byte{0x7c}, 16), 2
	a.rsc = [8]byte{0x01}
	n, ev, err = s.Handle(a.group1(), reply)
	if err != nil || ev != EventGroup {
		t.Fatalf("group message 1: event %d, %v", ev, err)
	}
	if !bytes.Equal(s.GTKRSC(), a.rsc[:]) {
		t.Errorf("new GTK RSC = %x, want %x", s.GTKRSC(), a.rsc)
	}
	a.readReply(reply[:n], infoVersionAES|infoMIC|infoSecure)
	if gtk, id := s.GTK(); !bytes.Equal(gtk, a.gtk) || id != 2 {
		t.Errorf("new GTK = %x id %d", gtk, id)
	}
	if mem.Heap.Stats().InUse != held {
		t.Errorf("group rekey changed memory in use from %d to %d", held, mem.Heap.Stats().InUse)
	}

	s.Close()
	if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
	}
}

func TestAttacks(t *testing.T) {
	tests := []struct {
		name    string
		attack  func(a *authenticator, s *Supplicant, reply []byte) error
		wantErr error
	}{
		{
			name: "message 3 with a bad MIC",
			attack: func(a *authenticator, s *Supplicant, reply []byte) error {
				m3 := a.message3(rsnElem)
				m3[offMIC] ^= 1
				_, _, err := s.Handle(m3, reply)
				return err
			},
			wantErr: ErrMIC,
		},
		{
			name: "message 3 with a new ANonce",
			attack: func(a *authenticator, s *Supplicant, reply []byte) error {
				a.anonce[0] ^= 1
				_, _, err := s.Handle(a.message3(rsnElem), reply)
				return err
			},
			wantErr: ErrNonce,
		},
		{
			name: "message 3 offers a weaker RSN element",
			attack: func(a *authenticator, s *Supplicant, reply []byte) error {
				weak := append([]byte(nil), rsnElem...)
				weak[13] = 2 // TKIP pairwise
				_, _, err := s.Handle(a.message3(weak), reply)
				return err
			},
			wantErr: ErrRSNMismatch,
		},
		{
			name: "replayed message 3",
			attack: func(a *authenticator, s *Supplicant, reply []byte) error {
				m3 := a.message3(rsnElem)
				if _, _, err := s.Handle(m3, reply); err != nil {
					return err
				}
				_, _, err := s.Handle(m3, reply)
				return err
			},
			wantErr: ErrReplay,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			s := newSupplicant(t, mem)
			a := newAuthenticator(t)
			reply := make([]byte, 256)
			n, _, err := s.Handle(a.message1(), reply)
			if err != nil {
				t.Fatal(err)
			}
			a.readMessage2(reply[:n])
			if err := tt.attack(a, s, reply); err != tt.wantErr {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			s.Close()
			if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
				t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
			}
		})
	}
}

func TestStateErrors(t *testing.T) {
	mem := newCheckedMemory(t)
	reply := make([]byte, 256)
	a := newAuthenticator(t)

	if _, err := New(nil, &counterRandom{}, []byte("password"), []byte("IEEE")); err != ErrNilMemory {
		t.Errorf("nil memory error = %v", err)
	}
	if _, err := New(mem, nil, []byte("password"), []byte("IEEE")); err != ErrNilRandom {
		t.Errorf("nil random error = %v", err)
	}

	s, _ := New(mem, failingRandom{}, []byte("password"), []byte("IEEE"))
	if _, _, err := s.Handle(a.message1(), reply); err != ErrNotStarted {
		t.Errorf("before Start error = %v, want ErrNotStarted", err)
	}
	s.Start(apAddr, staAddr, rsnElem, nil)
	if _, _, err := s.Handle(a.message1(), reply); err != ErrRandom {
		t.Errorf("failing random error = %v, want ErrRandom", err)
	}
	s.Close()

	s = newSupplicant(t, mem)
	a.ptk = make([]byte, 48)
	if _, _, err := s.Handle(a.message3(rsnElem), reply); err != ErrState {
		t.Errorf("message 3 before message 1 error = %v, want ErrState", err)
	}
	if _, _, err := s.Handle(a.message1()[:50], reply); err != ErrShort {
		t.Errorf("short frame error = %v, want ErrShort", err)
	}
	if _, _, err := s.Handle(a.message1(), reply[:20]); err != ErrBuffer {
		t.Errorf("small reply buffer error = %v, want ErrBuffer", err)
	}
	s.Close()
	if _, _, err := s.Handle(a.message1(), reply); err != ErrClosed {
		t.Errorf("after Close error = %v, want ErrClosed", err)
	}
	if mem.Heap.Stats().InUse != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.Heap.Stats().InUse, mem.dirtyFrees)
	}
}

func TestNewPSK(t *testing.T) {
	mem := newCheckedMemory(t)
	s, err := NewPSK(mem, &counterRandom{}, pbkdf2([]byte("password"), []byte("IEEE"), 4096, 32))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(apAddr, staAddr, rsnElem, rsnElem); err != nil {
		t.Fatal(err)
	}
	a := newAuthenticator(t)
	reply := make([]byte, 256)
	n, _, err := s.Handle(a.message1(), reply)
	if err != nil {
		t.Fatal(err)
	}
	a.readMessage2(reply[:n])
	if _, ev, err := s.Handle(a.message3(rsnElem), reply); err != nil || ev != EventPairwise {
		t.Fatalf("message 3: event %d, %v", ev, err)
	}
	if !bytes.Equal(s.TK(), a.ptk[32:48]) {
		t.Errorf("TK = %x, want %x", s.TK(), a.ptk[32:48])
	}
	s.Close()
	if _, err := NewPSK(mem, &counterRandom{}, make([]byte, 31)); err == nil {
		t.Error("NewPSK accepts a 31-byte key")
	}
}
