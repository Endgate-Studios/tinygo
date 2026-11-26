package wpa

import (
	"bytes"
	stdaes "crypto/aes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"testing"
	"unsafe"

	"runtime/memory/arena"
	"tinygo/crypto/aes"
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

func (m *checkedMemory) inUse() uintptr { return m.Heap.Stats().InUse }

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHMACSHA1(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
		data string
		want string
	}{
		{name: "RFC 2202 case 1", key: bytes.Repeat([]byte{0x0b}, 20), data: "Hi There", want: "b617318655057264e28bc0b6fb378c8ef146be00"},
		{name: "RFC 2202 case 2", key: []byte("Jefe"), data: "what do ya want for nothing?", want: "effcdf6ae5eb2fa2d27416d5f184df9c259a7c79"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mac
			m.init(tt.key)
			got := m.sum([]byte(tt.data), nil, nil, nil)
			if hex.EncodeToString(got[:]) != tt.want {
				t.Errorf("HMAC = %x, want %s", got, tt.want)
			}
		})
	}
}

func TestPBKDF2(t *testing.T) {
	tests := []struct {
		iterations int
		want       string
	}{
		{iterations: 1, want: "0c60c80f961f0e71f3a9b524af6012062fe037a6"},
		{iterations: 2, want: "ea6c014dc72d6f8ccd1ed92ace1d41f0d8de8957"},
		{iterations: 4096, want: "4b007901b765489abead49d926f721d065a429c1"},
	}
	for _, tt := range tests {
		out := make([]byte, 20)
		pbkdf2SHA1([]byte("password"), []byte("salt"), tt.iterations, out)
		if hex.EncodeToString(out) != tt.want {
			t.Errorf("RFC 6070 c=%d: %x, want %s", tt.iterations, out, tt.want)
		}
	}
}

func TestSetPassphrase(t *testing.T) {
	tests := []struct {
		name       string
		passphrase string
		ssid       string
		want       string
		wantErr    error
	}{
		{name: "IEEE 802.11 J.4.2 vector 1", passphrase: "password", ssid: "IEEE", want: "f42c6fc52df0ebef9ebb4b90b38a5f902e83fe1b135a70e23aed762e9710a12e"},
		{name: "IEEE 802.11 J.4.2 vector 2", passphrase: "ThisIsAPassword", ssid: "ThisIsASSID", want: "0dc0d6eb90555ed6419756b9a15ec3e3209b63df707dd508d14581f8982721af"},
		{name: "short passphrase", passphrase: "short", ssid: "x", wantErr: ErrPassphrase},
		{name: "empty SSID", passphrase: "password", ssid: "", wantErr: ErrSSID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			k, err := New(mem)
			if err != nil {
				t.Fatal(err)
			}
			err = k.SetPassphrase([]byte(tt.passphrase), []byte(tt.ssid))
			if err != tt.wantErr {
				t.Fatalf("SetPassphrase error = %v, want %v", err, tt.wantErr)
			}
			if err == nil {
				if got := hex.EncodeToString(k.pmk); got != tt.want {
					t.Errorf("PMK = %s, want %s", got, tt.want)
				}
				if mem.inUse() == 0 {
					t.Error("the PMK is not held in the injected memory")
				}
			}
			k.Close()
			if mem.inUse() != 0 || mem.dirtyFrees != 0 {
				t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.inUse(), mem.dirtyFrees)
			}
		})
	}
}

// refPRF is the IEEE 802.11 PRF built on crypto/hmac, as an independent reference.
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

func TestDerivePTK(t *testing.T) {
	mem := newCheckedMemory(t)
	k, err := New(mem)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.DerivePTK([6]byte{}, [6]byte{}, [32]byte{}, [32]byte{}); err != ErrNoPMK {
		t.Fatalf("DerivePTK without PMK error = %v, want ErrNoPMK", err)
	}
	if err := k.SetPassphrase([]byte("password"), []byte("IEEE")); err != nil {
		t.Fatal(err)
	}

	aa := [6]byte{0x00, 0x0c, 0x41, 0xd2, 0x94, 0xfb}
	spa := [6]byte{0x00, 0x0d, 0x3a, 0x26, 0x10, 0xfb}
	var anonce, snonce [32]byte
	for i := range anonce {
		anonce[i], snonce[i] = byte(i), byte(0xff-i)
	}
	if err := k.DerivePTK(aa, spa, anonce, snonce); err != nil {
		t.Fatal(err)
	}
	var data []byte
	data = append(append(data, aa[:]...), spa[:]...)
	data = append(append(data, anonce[:]...), snonce[:]...)
	want := refPRF(k.pmk, "Pairwise key expansion", data, PTKLen)
	if !bytes.Equal(k.ptk, want) {
		t.Fatalf("PTK = %x\nwant  %x", k.ptk, want)
	}
	if !bytes.Equal(k.KCK(), want[:16]) || !bytes.Equal(k.KEK(), want[16:32]) || !bytes.Equal(k.TK(), want[32:]) {
		t.Error("KCK, KEK or TK is the wrong part of the PTK")
	}

	first := append([]byte(nil), k.ptk...)
	if err := k.DerivePTK(spa, aa, snonce, anonce); err != nil || !bytes.Equal(k.ptk, first) {
		t.Error("the PTK depends on the order of the addresses or nonces")
	}

	held := mem.inUse()
	for i := 0; i < 5; i++ {
		if err := k.DerivePTK(aa, spa, anonce, snonce); err != nil {
			t.Fatal(err)
		}
	}
	if mem.inUse() != held {
		t.Errorf("repeated DerivePTK changed memory in use from %d to %d", held, mem.inUse())
	}
	k.Close()
	if mem.inUse() != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.inUse(), mem.dirtyFrees)
	}
	if err := k.DerivePTK(aa, spa, anonce, snonce); err != ErrClosed {
		t.Errorf("DerivePTK after Close error = %v, want ErrClosed", err)
	}
}

func TestMIC(t *testing.T) {
	mem := newCheckedMemory(t)
	k, _ := New(mem)
	var mic [MICLen]byte
	if err := k.MIC([]byte("frame"), &mic); err != ErrNoPTK {
		t.Fatalf("MIC without PTK error = %v, want ErrNoPTK", err)
	}
	if err := k.SetPassphrase([]byte("password"), []byte("IEEE")); err != nil {
		t.Fatal(err)
	}
	if err := k.DerivePTK([6]byte{1}, [6]byte{2}, [32]byte{3}, [32]byte{4}); err != nil {
		t.Fatal(err)
	}
	frame := bytes.Repeat([]byte{0xa5}, 300)
	if err := k.MIC(frame, &mic); err != nil {
		t.Fatal(err)
	}
	h := hmac.New(sha1.New, k.KCK())
	h.Write(frame)
	if want := h.Sum(nil)[:MICLen]; !bytes.Equal(mic[:], want) {
		t.Errorf("MIC = %x, want %x", mic, want)
	}
	k.Close()
	if mem.inUse() != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.inUse(), mem.dirtyFrees)
	}
}

func TestOutOfMemory(t *testing.T) {
	h, err := arena.NewHeap(make([]byte, 128))
	if err != nil {
		t.Fatal(err)
	}
	// Fill the heap, so that the PMK does not fit.
	var blocks []unsafe.Pointer
	for p := h.Alloc(16, 0); p != nil; p = h.Alloc(16, 0) {
		blocks = append(blocks, p)
	}
	k, _ := New(h)
	if err := k.SetPassphrase([]byte("password"), []byte("IEEE")); err != ErrNoMemory {
		t.Errorf("SetPassphrase in a full heap error = %v, want ErrNoMemory", err)
	}
	k.Close()
	for _, p := range blocks {
		h.Free(p)
	}
	if h.Stats().InUse != 0 {
		t.Errorf("memory in use after Close = %d, want 0", h.Stats().InUse)
	}
}

func TestNewRejectsNilMemory(t *testing.T) {
	if _, err := New(nil); err != ErrNilMemory {
		t.Errorf("New(nil) error = %v, want ErrNilMemory", err)
	}
}

func TestUnwrapRFC3394(t *testing.T) {
	mem := newCheckedMemory(t)
	c, err := aes.New(mem, unhex(t, "000102030405060708090a0b0c0d0e0f"))
	if err != nil {
		t.Fatal(err)
	}
	wrapped := unhex(t, "1fa68b0a8112b447aef34bd8fb5a7b829d3e862371d2cfe5")
	out := make([]byte, 16)
	if !unwrap(c, wrapped, out) {
		t.Fatal("RFC 3394 4.1 integrity check failed")
	}
	if got := hex.EncodeToString(out); got != "00112233445566778899aabbccddeeff" {
		t.Errorf("RFC 3394 4.1 key = %s", got)
	}
	wrapped[3] ^= 1
	if unwrap(c, wrapped, out) {
		t.Error("a changed wrap passed the integrity check")
	}
	c.Close()
}

func TestWrapRFC3394(t *testing.T) {
	mem := newCheckedMemory(t)
	c, err := aes.New(mem, unhex(t, "000102030405060708090a0b0c0d0e0f"))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 24)
	wrap(c, unhex(t, "00112233445566778899aabbccddeeff"), out)
	if got := hex.EncodeToString(out); got != "1fa68b0a8112b447aef34bd8fb5a7b829d3e862371d2cfe5" {
		t.Errorf("RFC 3394 4.1 wrap = %s", got)
	}
	plain := make([]byte, 16)
	if !unwrap(c, out, plain) || hex.EncodeToString(plain) != "00112233445566778899aabbccddeeff" {
		t.Error("unwrap of the wrap failed")
	}
	c.Close()
}

// refWrap is RFC 3394 key wrap built on crypto/aes, to make test input.
func refWrap(kek, key []byte) []byte {
	b, _ := stdaes.NewCipher(kek)
	n := len(key) / 8
	a := uint64(keyWrapIV)
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

func TestUnwrapKey(t *testing.T) {
	mem := newCheckedMemory(t)
	k, _ := New(mem)
	gtk := bytes.Repeat([]byte{0x5a}, 16)
	out := make([]byte, 16)
	if err := k.UnwrapKey(make([]byte, 24), out); err != ErrNoPTK {
		t.Fatalf("UnwrapKey without PTK error = %v, want ErrNoPTK", err)
	}
	if err := k.SetPassphrase([]byte("password"), []byte("IEEE")); err != nil {
		t.Fatal(err)
	}
	if err := k.DerivePTK([6]byte{1}, [6]byte{2}, [32]byte{3}, [32]byte{4}); err != nil {
		t.Fatal(err)
	}
	held := mem.inUse()

	wrapped := refWrap(k.KEK(), gtk)
	if err := k.UnwrapKey(wrapped, out); err != nil || !bytes.Equal(out, gtk) {
		t.Fatalf("UnwrapKey = %x, %v, want %x", out, err, gtk)
	}
	if mem.inUse() != held {
		t.Errorf("UnwrapKey did not free its cipher: in use %d, want %d", mem.inUse(), held)
	}
	wrapped[9] ^= 1
	if err := k.UnwrapKey(wrapped, out); err != ErrUnwrap {
		t.Errorf("changed wrap error = %v, want ErrUnwrap", err)
	}
	if err := k.UnwrapKey(wrapped[:20], out); err != ErrWrappedSize {
		t.Errorf("short wrap error = %v, want ErrWrappedSize", err)
	}
	k.Close()
	if mem.inUse() != 0 || mem.dirtyFrees != 0 {
		t.Errorf("after Close: in use %d, blocks freed without zeroing %d", mem.inUse(), mem.dirtyFrees)
	}
}

func TestSetPMK(t *testing.T) {
	mem := newCheckedMemory(t)
	k, err := New(mem)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if err := k.SetPMK(make([]byte, 16)); err != ErrPMKLen {
		t.Errorf("short PMK: error %v, want %v", err, ErrPMKLen)
	}
	pmk := bytes.Repeat([]byte{0x5a}, PMKLen)
	if err := k.SetPMK(pmk); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(k.pmk, pmk) {
		t.Errorf("PMK = %x, want %x", k.pmk, pmk)
	}
}
