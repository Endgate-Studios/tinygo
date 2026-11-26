package sae

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"strings"
	"testing"

	"runtime/memory"
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

func newMem(t *testing.T) *arena.Heap {
	h, err := arena.NewHeap(make([]byte, 8<<10))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestAnnexJ10 uses the SAE test vector of IEEE 802.11-2020 Annex J.10, as
// the test_sae.c of the ESP-IDF v5.3.2 supplicant.
func TestAnnexJ10(t *testing.T) {
	mem := newMem(t)
	own := [6]byte{0x4d, 0x3f, 0x2f, 0xff, 0xe3, 0x87}
	peer := [6]byte{0xa5, 0xd8, 0xaa, 0x95, 0x8e, 0x3c}
	s := memory.New[state](mem)
	a := &SAE{mem: mem, s: s}
	if !derivePWE(s, own, peer, []byte("mekmitasdigoat")) {
		t.Fatal("no password element")
	}
	copy(s.rand[:], unhex(t, "992465fd3daa3c60aa6565b7f62a2a7f2e12dd12f198faf4fbed89d7ff1ace94"))
	mask := [32]byte(unhex(t, "9507a90f777a044d6a0830b91ea3d5dd70bece44e1acffb86983b5e1bf9fb322"))
	if err := a.commit(&mask); err != nil {
		t.Fatal(err)
	}
	var commit [CommitLen]byte
	if n, err := a.WriteCommit(commit[:], nil); err != nil || n != CommitLen {
		t.Fatal(n, err)
	}
	want := unhex(t, "1300 2e2c0f0db52440ad146d967114ce005ce1eab0aa2c2e5c2871b774f6c2575c65"+
		"d5ad9e00829707aa36ba8b859738fc961d08243505f47c035376d7ac4bc8d7b95083bf43827d0fc31ed778dd3671"+
		"fd21a46d1091d64b6f9a1e1272621325dbe1")
	if !bytes.Equal(commit[:], want) {
		t.Fatalf("commit\n got %x\nwant %x", commit, want)
	}
	peerCommit := unhex(t, "1300 591b96f3397fb945100848e7b550543b6720d88337ee93fc49fd6df7e08b5223"+
		"e71b9bb048d3873f20556953a96c91536fd8ee6ca9b4a68a148b056a909be03e83ae208f60f8ef5537858074db06"+
		"68703239986299 9b511e0a1552a5fea317c2")
	if err := a.ProcessCommit(peerCommit); err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "1e733f6d9bd5325628730433883 1b09a39406d121017073a5c30db36f36cb81a"); !bytes.Equal(s.kck[:], want) {
		t.Errorf("KCK %x, want %x", s.kck, want)
	}
	if want := unhex(t, "4e4dfab1a2dd8ac1a91790f953faaa452ae5c6873ab75b63605ba663f8a7fe59"); !bytes.Equal(a.PMK(), want) {
		t.Errorf("PMK %x, want %x", a.PMK(), want)
	}
	if want := unhex(t, "8747a600eea3f9f22475df58ca1e5498"); !bytes.Equal(a.PMKID(), want) {
		t.Errorf("PMKID %x, want %x", a.PMKID(), want)
	}
	a.Close()
	if mem.Stats().InUse != 0 {
		t.Errorf("memory in use after Close: %d", mem.Stats().InUse)
	}
}

// exchange runs SAE between a station and an access point and gives both.
func exchange(t *testing.T, staPass, apPass string) (*SAE, *SAE, error) {
	t.Helper()
	mem := newMem(t)
	rng := rand.New(rand.NewSource(7))
	sta, ap := [6]byte{2, 0, 0, 0, 0, 1}, [6]byte{2, 0, 0, 0, 0, 0xaa}
	a, err := New(mem, rng, sta, ap, []byte(staPass))
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(mem, rng, ap, sta, []byte(apPass))
	if err != nil {
		t.Fatal(err)
	}
	var ca, cb [CommitLen]byte
	a.WriteCommit(ca[:], nil)
	b.WriteCommit(cb[:], nil)
	if err := a.ProcessCommit(cb[:]); err != nil {
		t.Fatal(err)
	}
	if err := b.ProcessCommit(ca[:]); err != nil {
		t.Fatal(err)
	}
	var fa, fb [ConfirmLen]byte
	a.WriteConfirm(1, fa[:])
	b.WriteConfirm(1, fb[:])
	if err := a.VerifyConfirm(fb[:]); err != nil {
		return a, b, err
	}
	return a, b, b.VerifyConfirm(fa[:])
}

func TestExchange(t *testing.T) {
	a, b, err := exchange(t, "correct horse", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.PMK(), b.PMK()) || !bytes.Equal(a.PMKID(), b.PMKID()) {
		t.Error("the two sides derived different keys")
	}
	a.Close()
	b.Close()

	a, b, err = exchange(t, "correct horse", "wrong horse")
	if err != ErrConfirm {
		t.Errorf("wrong password: %v, want ErrConfirm", err)
	}
	a.Close()
	b.Close()
}

func TestBadCommit(t *testing.T) {
	mem := newMem(t)
	rng := rand.New(rand.NewSource(9))
	a, err := New(mem, rng, [6]byte{1}, [6]byte{2}, []byte("password"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var own [CommitLen]byte
	a.WriteCommit(own[:], nil)
	if err := a.ProcessCommit(own[:]); err != ErrReflected {
		t.Errorf("own commit: %v, want ErrReflected", err)
	}
	bad := own
	bad[CommitLen-1] ^= 1 // the element is not on the curve
	if err := a.ProcessCommit(bad[:]); err != ErrCommit {
		t.Errorf("point not on the curve: %v, want ErrCommit", err)
	}
	bad = own
	clear(bad[2:34]) // scalar 0
	if err := a.ProcessCommit(bad[:]); err != ErrCommit {
		t.Errorf("scalar 0: %v, want ErrCommit", err)
	}
	bad = own
	bad[0] = 20
	if err := a.ProcessCommit(bad[:]); err != ErrGroup {
		t.Errorf("group 20: %v, want ErrGroup", err)
	}
	var c [ConfirmLen]byte
	if _, err := a.WriteConfirm(1, c[:]); err != ErrState {
		t.Errorf("confirm before a commit: %v, want ErrState", err)
	}
}
