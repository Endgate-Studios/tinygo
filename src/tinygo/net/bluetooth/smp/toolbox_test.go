package smp

import (
	"encoding/hex"
	"strings"
	"testing"
	"unsafe"

	"runtime/memory/arena"
)

// checkedMemory is an arena heap that records whether each freed block was zeroed first.
type checkedMemory struct {
	*arena.Heap
	sizes      map[unsafe.Pointer]uintptr
	dirtyFrees int
}

func newCheckedMemory(t *testing.T) *checkedMemory {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 8192))
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

func b16(t *testing.T, s string) (v [16]byte) {
	t.Helper()
	d := unhex(t, s)
	if len(d) != 16 {
		t.Fatalf("want 16 bytes, got %d", len(d))
	}
	copy(v[:], d)
	return v
}

func b32(t *testing.T, s string) (v [32]byte) {
	t.Helper()
	d := unhex(t, s)
	if len(d) != 32 {
		t.Fatalf("want 32 bytes, got %d", len(d))
	}
	copy(v[:], d)
	return v
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	d, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCMACRFC4493(t *testing.T) {
	key := b16(t, "2b7e1516 28aed2a6 abf71588 09cf4f3c")
	msg := unhex(t, "6bc1bee2 2e409f96 e93d7e11 7393172a ae2d8a57 1e03ac9c 9eb76fac 45af8e51"+
		" 30c81c46 a35ce411 e5fbc119 1a0a52ef f69f2445 df4f9b17 ad2b417b e66c3710")
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "example 1, empty", n: 0, want: "bb1d6929 e9593728 7fa37d12 9b756746"},
		{name: "example 2, 16 bytes", n: 16, want: "070a16b4 6b4d4144 f79bdd9d d04a287c"},
		{name: "example 3, 40 bytes", n: 40, want: "dfa66747 de9ae630 30ca3261 1497c827"},
		{name: "example 4, 64 bytes", n: 64, want: "51f0bebf 7e3b9d92 fc497417 79363cfe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mem := newCheckedMemory(t)
			var out [16]byte
			if err := CMAC(mem, &key, msg[:tt.n], &out); err != nil {
				t.Fatal(err)
			}
			if want := b16(t, tt.want); out != want {
				t.Errorf("CMAC = %x, want %x", out, want)
			}
			mem.check(t)
		})
	}
}

func TestC1AndS1(t *testing.T) {
	mem := newCheckedMemory(t)
	var k, out [16]byte
	// Core spec v5.4 Vol 3 Part H 2.2.3 example.
	r := b16(t, "5783D52156AD6F0E6388274EC6702EE0")
	var preq, pres [7]byte
	copy(preq[:], unhex(t, "07071000000101"))
	copy(pres[:], unhex(t, "05000800000302"))
	var ia, ra [6]byte
	copy(ia[:], unhex(t, "A1A2A3A4A5A6"))
	copy(ra[:], unhex(t, "B1B2B3B4B5B6"))
	if err := C1(mem, &k, &r, &preq, &pres, 1, 0, &ia, &ra, &out); err != nil {
		t.Fatal(err)
	}
	if want := b16(t, "1e1e3fef878988ead2a74dc5bef13b86"); out != want {
		t.Errorf("c1 = %x, want %x", out, want)
	}

	// Core spec v5.4 Vol 3 Part H 2.2.4 example.
	r1 := b16(t, "000F0E0D0C0B0A091122334455667788")
	r2 := b16(t, "010203040506070899AABBCCDDEEFF00")
	if err := S1(mem, &k, &r1, &r2, &out); err != nil {
		t.Fatal(err)
	}
	if want := b16(t, "9a1fe1f0e8b0f49b5b4216ae796da062"); out != want {
		t.Errorf("s1 = %x, want %x", out, want)
	}
	mem.check(t)
}

// Sample data of Core spec v5.4 Vol 3 Part H appendix D.
const (
	sampleU  = "20b003d2 f297be2c 5e2c83a7 e9f9a5b9 eff49111 acf4fddb cc030148 0e359de6"
	sampleV  = "55188b3d 32f6bb9a 900afcfb eed4e72a 59cb9ac2 f19d7cfb 6b4fdd49 f47fc5fd"
	sampleX  = "d5cb8454 d177733e ffffb2ec 712baeab"
	sampleN2 = "a6e8e7cc 25a75f6e 216583f7 ff3dc4cf"
	sampleW  = "ec0234a3 57c8ad05 341010a6 0a397d9b 99796b13 b4f866f1 868d34f3 73bfa698"
	sampleA1 = "00561237 37bfce"
	sampleA2 = "00a71370 2dcfc1"
)

func TestF4(t *testing.T) {
	mem := newCheckedMemory(t)
	u, v, x := b32(t, sampleU), b32(t, sampleV), b16(t, sampleX)
	var out [16]byte
	if err := F4(mem, &u, &v, &x, 0, &out); err != nil {
		t.Fatal(err)
	}
	if want := b16(t, "f2c916f1 07a9bd1c f1eda1be a974872d"); out != want {
		t.Errorf("f4 = %x, want %x", out, want)
	}
	mem.check(t)
}

func TestF5(t *testing.T) {
	mem := newCheckedMemory(t)
	w := b32(t, sampleW)
	n1, n2 := b16(t, sampleX), b16(t, sampleN2)
	var a1, a2 [7]byte
	copy(a1[:], unhex(t, sampleA1))
	copy(a2[:], unhex(t, sampleA2))
	var macKey, ltk [16]byte
	if err := F5(mem, &w, &n1, &n2, &a1, &a2, &macKey, &ltk); err != nil {
		t.Fatal(err)
	}
	if want := b16(t, "2965f176 a1084a02 fd3f6a20 ce636e20"); macKey != want {
		t.Errorf("MacKey = %x, want %x", macKey, want)
	}
	if want := b16(t, "69867911 69d7cd23 980522b5 94750a38"); ltk != want {
		t.Errorf("LTK = %x, want %x", ltk, want)
	}
	mem.check(t)
}

func TestF6(t *testing.T) {
	mem := newCheckedMemory(t)
	w := b16(t, "2965f176 a1084a02 fd3f6a20 ce636e20")
	n1, n2 := b16(t, sampleX), b16(t, sampleN2)
	r := b16(t, "12a3343b b453bb54 08da42d2 0c2d0fc8")
	ioCap := [3]byte{0x01, 0x01, 0x02}
	var a1, a2 [7]byte
	copy(a1[:], unhex(t, sampleA1))
	copy(a2[:], unhex(t, sampleA2))
	var out [16]byte
	if err := F6(mem, &w, &n1, &n2, &r, &ioCap, &a1, &a2, &out); err != nil {
		t.Fatal(err)
	}
	if want := b16(t, "e3c47398 9cd0e8c5 d26c0b09 da958f61"); out != want {
		t.Errorf("f6 = %x, want %x", out, want)
	}
	mem.check(t)
}

func TestG2(t *testing.T) {
	mem := newCheckedMemory(t)
	u, v := b32(t, sampleU), b32(t, sampleV)
	x, y := b16(t, sampleX), b16(t, sampleN2)
	got, err := G2(mem, &u, &v, &x, &y)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x2f9ed5ba {
		t.Errorf("g2 = %#08x, want 0x2f9ed5ba", got)
	}
	mem.check(t)
}

func TestNilMemory(t *testing.T) {
	var k, out [16]byte
	if err := CMAC(nil, &k, nil, &out); err != ErrNilMemory {
		t.Errorf("CMAC(nil memory) error = %v, want ErrNilMemory", err)
	}
	tiny, _ := arena.NewHeap(make([]byte, 64))
	if err := CMAC(tiny, &k, nil, &out); err != ErrNoMemory {
		t.Errorf("CMAC with a tiny heap error = %v, want ErrNoMemory", err)
	}
}
