package p256

import (
	"encoding/hex"
	"errors"
	"math/big"
	"math/rand"
	"strings"
	"testing"
	"unsafe"

	"runtime/memory/arena"
)

var bigP, _ = new(big.Int).SetString("ffffffff00000001000000000000000000000000ffffffffffffffffffffffff", 16)

// checkedMemory is an arena heap that records whether each freed block was zeroed first.
type checkedMemory struct {
	*arena.Heap
	sizes      map[unsafe.Pointer]uintptr
	dirtyFrees int
}

func newCheckedMemory(t *testing.T) *checkedMemory {
	t.Helper()
	h, err := arena.NewHeap(make([]byte, 2048))
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

func toBig(e *element) *big.Int {
	var b [32]byte
	feToBytes(&b, e)
	return new(big.Int).SetBytes(b[:])
}

func fromBig(t *testing.T, v *big.Int) element {
	t.Helper()
	var b [32]byte
	v.FillBytes(b[:])
	var e element
	if !feFromBytes(&e, &b) {
		t.Fatalf("%x is not below p", v)
	}
	return e
}

func TestFieldAgainstBig(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	edges := []*big.Int{
		big.NewInt(0), big.NewInt(1), big.NewInt(2),
		new(big.Int).Sub(bigP, big.NewInt(1)), new(big.Int).Sub(bigP, big.NewInt(2)),
		new(big.Int).Lsh(big.NewInt(1), 255), new(big.Int).Lsh(big.NewInt(1), 224),
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 224), big.NewInt(1)),
	}
	random := func() *big.Int { return new(big.Int).Rand(rng, bigP) }
	// The cases are made one at a time, so the board heap holds only one pair of values.
	const randomCases = 20000
	n := len(edges)*len(edges) + randomCases
	for i := 0; i < n; i++ {
		var c [2]*big.Int
		if i < len(edges)*len(edges) {
			c = [2]*big.Int{edges[i/len(edges)], edges[i%len(edges)]}
		} else {
			c = [2]*big.Int{random(), random()}
		}
		a, b := fromBig(t, c[0]), fromBig(t, c[1])
		var r element
		feMul(&r, &a, &b)
		if want := new(big.Int).Mod(new(big.Int).Mul(c[0], c[1]), bigP); toBig(&r).Cmp(want) != 0 {
			t.Fatalf("%x * %x = %x, want %x", c[0], c[1], toBig(&r), want)
		}
		feAdd(&r, &a, &b)
		if want := new(big.Int).Mod(new(big.Int).Add(c[0], c[1]), bigP); toBig(&r).Cmp(want) != 0 {
			t.Fatalf("%x + %x = %x, want %x", c[0], c[1], toBig(&r), want)
		}
		feSub(&r, &a, &b)
		if want := new(big.Int).Mod(new(big.Int).Sub(c[0], c[1]), bigP); toBig(&r).Cmp(want) != 0 {
			t.Fatalf("%x - %x = %x, want %x", c[0], c[1], toBig(&r), want)
		}
	}
	for i := 0; i < 50; i++ {
		v := random()
		if v.Sign() == 0 {
			continue
		}
		a := fromBig(t, v)
		var inv, one element
		feInv(&inv, &a)
		feMul(&one, &inv, &a)
		if one != (element{1}) {
			t.Fatalf("%x * inverse = %x, want 1", v, toBig(&one))
		}
	}
}

func TestGeneratorAndOrder(t *testing.T) {
	if !onCurve(&gen.x, &gen.y) {
		t.Fatal("G is not on the curve")
	}
	var acc, tmp point
	var table [16]point
	nMinus1 := n
	nMinus1[31]--
	scalarMult(&acc, &tmp, &table, &gen, &n)
	var x, y element
	if acc.affine(&x, &y) {
		t.Error("n * G is not the identity")
	}
	scalarMult(&acc, &tmp, &table, &gen, &nMinus1)
	if !acc.affine(&x, &y) || x != gen.x {
		t.Error("(n-1) * G is not -G")
	}
}

// RFC 5903 section 8.1, the 256-bit random ECP group.
func TestRFC5903(t *testing.T) {
	mem := newCheckedMemory(t)
	i := b32(t, "C88F01F5 10D9AC3F 70A292DA A2316DE5 44E9AAB8 AFE84049 C62A9C57 862D1433")
	r := b32(t, "C6EF9C5D 78AE012A 011164AC B397CE20 88685D8F 06BF9BE0 B283AB46 476BEE53")
	var gix, giy, grx, gry, shared [32]byte
	if err := PublicKey(mem, &i, &gix, &giy); err != nil {
		t.Fatal(err)
	}
	if gix != b32(t, "DAD0B653 94221CF9 B051E1FE CA5787D0 98DFE637 FC90B9EF 945D0C37 72581180") ||
		giy != b32(t, "5271A046 1CDB8252 D61F1C45 6FA3E59A B1F45B33 ACCF5F58 389E0577 B8990BB3") {
		t.Errorf("g^i = %x, %x", gix, giy)
	}
	if err := PublicKey(mem, &r, &grx, &gry); err != nil {
		t.Fatal(err)
	}
	if err := SharedSecret(mem, &i, &grx, &gry, &shared); err != nil {
		t.Fatal(err)
	}
	if shared != b32(t, "D6840F6B 42F6EDAF D13116E0 E1256520 2FEF8E9E CE7DCE03 812464D0 4B9442DE") {
		t.Errorf("g^ir = %x", shared)
	}
	mem.check(t)
}

func TestAgainstCryptoECDH(t *testing.T) {
	mem := newCheckedMemory(t)
	rng := rand.New(rand.NewSource(2))
	for k := 0; k < 100; k++ {
		ref, err := ecdhKey(rng)
		if err != nil {
			t.Fatal(err)
		}
		peer, _ := ecdhKey(rng)
		var priv, x, y, px, py, shared [32]byte
		copy(priv[:], ref.Bytes())
		if err := PublicKey(mem, &priv, &x, &y); err != nil {
			t.Fatal(err)
		}
		pub := ref.PublicKey().Bytes() // 0x04 || X || Y
		if string(x[:]) != string(pub[1:33]) || string(y[:]) != string(pub[33:]) {
			t.Fatalf("key %d: public key differs from crypto/ecdh", k)
		}
		pp := peer.PublicKey().Bytes()
		copy(px[:], pp[1:33])
		copy(py[:], pp[33:])
		if err := SharedSecret(mem, &priv, &px, &py, &shared); err != nil {
			t.Fatal(err)
		}
		want, _ := ref.ECDH(peer.PublicKey())
		if string(shared[:]) != string(want) {
			t.Fatalf("key %d: shared secret differs from crypto/ecdh", k)
		}
	}
	mem.check(t)
}

func TestRejects(t *testing.T) {
	mem := newCheckedMemory(t)
	var priv, x, y, out [32]byte
	priv[31] = 7
	if err := PublicKey(mem, &priv, &x, &y); err != nil {
		t.Fatal(err)
	}
	badY := y
	badY[31] ^= 1
	var p [32]byte
	pb := bigP.Bytes()
	copy(p[:], pb)
	tests := []struct {
		name   string
		px, py [32]byte
	}{
		{name: "y changed", px: x, py: badY},
		{name: "origin", px: [32]byte{}, py: [32]byte{}},
		{name: "x equal to p", px: p, py: y},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SharedSecret(mem, &priv, &tt.px, &tt.py, &out); err != ErrPoint {
				t.Errorf("error = %v, want ErrPoint", err)
			}
			if OnCurve(&tt.px, &tt.py) {
				t.Error("OnCurve accepted the point")
			}
		})
	}
	if !OnCurve(&x, &y) {
		t.Error("OnCurve rejected a real public key")
	}

	nMinus1, nPlus1 := n, n
	nMinus1[31]--
	nPlus1[31]++
	scalars := []struct {
		name string
		k    [32]byte
		want error
	}{
		{name: "zero", k: [32]byte{}, want: ErrScalar},
		{name: "n", k: n, want: ErrScalar},
		{name: "n+1", k: nPlus1, want: ErrScalar},
		{name: "n-1", k: nMinus1, want: nil},
	}
	for _, sc := range scalars {
		if err := PublicKey(mem, &sc.k, &x, &y); err != sc.want {
			t.Errorf("scalar %s: error = %v, want %v", sc.name, err, sc.want)
		}
	}
	if err := PublicKey(nil, &priv, &x, &y); err != ErrNilMemory {
		t.Errorf("nil memory error = %v", err)
	}
	mem.check(t)
}

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

func TestGenerateKey(t *testing.T) {
	mem := newCheckedMemory(t)
	var priv, x, y [32]byte
	if err := GenerateKey(mem, &counterRandom{}, &priv, &x, &y); err != nil {
		t.Fatal(err)
	}
	if !OnCurve(&x, &y) {
		t.Error("generated public key is not on the curve")
	}
	if err := GenerateKey(mem, failingRandom{}, &priv, &x, &y); err != ErrRandom || priv != ([32]byte{}) {
		t.Errorf("failing random: error %v, private key not cleared %v", err, priv != [32]byte{})
	}
	mem.check(t)
}

func b32(t *testing.T, s string) (v [32]byte) {
	t.Helper()
	d, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil || len(d) != 32 {
		t.Fatalf("bad 32-byte value %q", s)
	}
	copy(v[:], d)
	return v
}

func BenchmarkSharedSecret(b *testing.B) {
	h, _ := arena.NewHeap(make([]byte, 4096))
	var priv, x, y, out [32]byte
	priv[31] = 7
	PublicKey(h, &priv, &x, &y)
	priv[0] = 0x5a
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SharedSecret(h, &priv, &x, &y, &out)
	}
}

// TestDoubleMatchesAdd checks the doubling formula against the complete addition, in affine form.
func TestDoubleMatchesAdd(t *testing.T) {
	p := gen
	for i := 0; i < 200; i++ {
		var d, a point
		d.double(&p)
		a.add(&p, &p)
		var dx, dy, ax, ay element
		if !d.affine(&dx, &dy) || !a.affine(&ax, &ay) || dx != ax || dy != ay {
			t.Fatalf("step %d: double differs from add", i)
		}
		p.add(&p, &gen) // the next point
	}
	var id, d point
	id.setIdentity()
	d.double(&id)
	var x, y element
	if d.affine(&x, &y) {
		t.Error("double of the identity is not the identity")
	}
}
