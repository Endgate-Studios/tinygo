package srp

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"
)

func unhex(t testing.TB, s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestVectors runs each exchange of the Python oracle and checks every
// intermediate value.
func TestVectors(t *testing.T) {
	for _, tc := range vectors {
		t.Run(tc.name, func(t *testing.T) {
			user, pass := []byte(tc.username), []byte(tc.password)
			salt := (*[SaltSize]byte)(unhex(t, tc.salt))
			a := (*[SecretSize]byte)(unhex(t, tc.a))
			b := (*[SecretSize]byte)(unhex(t, tc.b))

			var v [PublicSize]byte
			Verifier(&v, user, pass, salt)
			if hex.EncodeToString(v[:]) != tc.v {
				t.Fatalf("v = %x", v)
			}
			var s Server
			if err := s.InitPassword(user, pass, salt, b); err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(s.PublicKey()[:]) != tc.B {
				t.Fatalf("B = %x", s.PublicKey()[:])
			}
			if *s.Salt() != *salt {
				t.Fatal("salt changed")
			}
			var c Client
			if err := c.Init(a); err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(c.PublicKey()[:]) != tc.A {
				t.Fatalf("A = %x", c.PublicKey()[:])
			}

			var u [hashWords]uint32
			scramble(&u, c.PublicKey()[:], s.PublicKey()[:])
			var ub [64]byte
			for i, w := range u {
				for j := 0; j < 4; j++ {
					ub[63-4*i-j] = byte(w >> (8 * j))
				}
			}
			if hex.EncodeToString(ub[:]) != tc.u {
				t.Fatalf("u = %x", ub)
			}
			var sb [PublicSize]byte
			save := s
			if !save.premaster(c.PublicKey()[:]) {
				t.Fatal("premaster rejects A")
			}
			toBytes(&sb, &save.w[1])
			if hex.EncodeToString(sb[:]) != tc.S {
				t.Fatalf("S = %x", sb)
			}

			var m1, m2 [ProofSize]byte
			var ck, sk [KeySize]byte
			if err := c.Proof(user, pass, salt, s.PublicKey()[:], &m1, &ck); err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(m1[:]) != tc.M1 {
				t.Fatalf("M1 = %x", m1)
			}
			if hex.EncodeToString(ck[:]) != tc.K {
				t.Fatalf("client K = %x", ck)
			}
			pa := *c.PublicKey()
			if err := s.Verify(pa[:], &m1, &m2, &sk); err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(m2[:]) != tc.M2 {
				t.Fatalf("M2 = %x", m2)
			}
			if sk != ck {
				t.Fatalf("server K = %x", sk)
			}
			if err := c.Verify(&m2); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestVerifierInit checks Init with a verifier against InitPassword.
func TestVerifierInit(t *testing.T) {
	tc := vectors[0]
	var s Server
	v := (*[PublicSize]byte)(unhex(t, tc.v))
	if err := s.Init([]byte(tc.username), v, (*[SaltSize]byte)(unhex(t, tc.salt)), (*[SecretSize]byte)(unhex(t, tc.b))); err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(s.PublicKey()[:]) != tc.B {
		t.Fatalf("B = %x", s.PublicKey()[:])
	}
}

func nBytes() []byte {
	var b [PublicSize]byte
	toBytes(&b, &n)
	return b[:]
}

// TestServerFailures runs exchanges that the server must reject.
func TestServerFailures(t *testing.T) {
	tc := vectors[0]
	salt := (*[SaltSize]byte)(unhex(t, tc.salt))
	a := (*[SecretSize]byte)(unhex(t, tc.a))
	b := (*[SecretSize]byte)(unhex(t, tc.b))
	user, pass := []byte(tc.username), []byte(tc.password)
	nPlus1 := nBytes()
	nPlus1[PublicSize-1]++

	for _, f := range []struct {
		name     string
		password string              // the client's password
		a        func([]byte) []byte // changes A
		m1       func(*[ProofSize]byte)
		init     bool // Init the server
		want     error
	}{
		{name: "success", password: tc.password, init: true},
		{name: "short A", password: tc.password, init: true, a: func(a []byte) []byte { return a[1:] }, want: ErrProof},
		{name: "wrong password", password: "1234", init: true, want: ErrProof},
		{name: "tampered M1", password: tc.password, init: true, m1: func(m *[ProofSize]byte) { m[7] ^= 0x10 }, want: ErrProof},
		{name: "A = 0", password: tc.password, init: true, a: func([]byte) []byte { return make([]byte, PublicSize) }, want: ErrPublicKey},
		{name: "A = 0, one byte", password: tc.password, init: true, a: func([]byte) []byte { return []byte{0} }, want: ErrPublicKey},
		{name: "A = N", password: tc.password, init: true, a: func([]byte) []byte { return nBytes() }, want: ErrPublicKey},
		{name: "A = N+1", password: tc.password, init: true, a: func([]byte) []byte { return nPlus1 }, want: ErrProof},
		{name: "A empty", password: tc.password, init: true, a: func([]byte) []byte { return nil }, want: ErrPublicKey},
		{name: "A too long", password: tc.password, init: true, a: func(a []byte) []byte { return append([]byte{0}, a...) }, want: ErrPublicKey},
		{name: "no Init", password: tc.password, want: ErrState},
	} {
		t.Run(f.name, func(t *testing.T) {
			var s Server
			if f.init {
				if err := s.InitPassword(user, pass, salt, b); err != nil {
					t.Fatal(err)
				}
			}
			var c Client
			c.Init(a)
			var m1, m2 [ProofSize]byte
			var ck, sk [KeySize]byte
			if err := c.Proof(user, []byte(f.password), salt, s.PublicKey()[:], &m1, &ck); err != nil && f.init {
				t.Fatal(err)
			}
			pa := append([]byte(nil), c.PublicKey()[:]...)
			if f.a != nil {
				pa = f.a(pa)
			}
			if f.m1 != nil {
				f.m1(&m1)
			}
			err := s.Verify(pa, &m1, &m2, &sk)
			if err != f.want {
				t.Fatalf("Verify: %v, want %v", err, f.want)
			}
			var zero [KeySize]byte
			if err != nil && (sk != zero || m2 != zero) {
				t.Fatal("output written on failure")
			}
			if err == nil && sk != ck {
				t.Fatal("keys differ")
			}
			// The exchange is over either way.
			if err := s.Verify(pa, &m1, &m2, &sk); err != ErrState {
				t.Fatalf("second Verify: %v, want %v", err, ErrState)
			}
		})
	}
}

// TestClientFailures runs exchanges that the client must reject.
func TestClientFailures(t *testing.T) {
	tc := vectors[0]
	salt := (*[SaltSize]byte)(unhex(t, tc.salt))
	a := (*[SecretSize]byte)(unhex(t, tc.a))
	b := (*[SecretSize]byte)(unhex(t, tc.b))
	user, pass := []byte(tc.username), []byte(tc.password)
	for _, f := range []struct {
		name string
		b    func([]byte) []byte
		m2   func(*[ProofSize]byte)
		want error // from Proof, else from Verify
	}{
		{name: "success"},
		{name: "B = 0", b: func([]byte) []byte { return make([]byte, PublicSize) }, want: ErrPublicKey},
		{name: "B = N", b: func([]byte) []byte { return nBytes() }, want: ErrPublicKey},
		{name: "B too long", b: func(b []byte) []byte { return append([]byte{1}, b...) }, want: ErrPublicKey},
		{name: "tampered M2", m2: func(m *[ProofSize]byte) { m[63] ^= 1 }, want: ErrProof},
	} {
		t.Run(f.name, func(t *testing.T) {
			var s Server
			s.InitPassword(user, pass, salt, b)
			var c Client
			c.Init(a)
			pb := append([]byte(nil), s.PublicKey()[:]...)
			if f.b != nil {
				pb = f.b(pb)
			}
			var m1, m2 [ProofSize]byte
			var ck, sk [KeySize]byte
			if err := c.Proof(user, pass, salt, pb, &m1, &ck); err != nil {
				if err != f.want {
					t.Fatalf("Proof: %v, want %v", err, f.want)
				}
				return
			}
			pa := *c.PublicKey()
			if err := s.Verify(pa[:], &m1, &m2, &sk); err != nil {
				t.Fatal(err)
			}
			if f.m2 != nil {
				f.m2(&m2)
			}
			if err := c.Verify(&m2); err != f.want {
				t.Fatalf("Verify: %v, want %v", err, f.want)
			}
			if err := c.Verify(&m2); err != ErrState {
				t.Fatalf("second Verify: %v, want %v", err, ErrState)
			}
		})
	}
}

// TestInitFailures checks the arguments that Init rejects.
func TestInitFailures(t *testing.T) {
	tc := vectors[0]
	salt := (*[SaltSize]byte)(unhex(t, tc.salt))
	b := (*[SecretSize]byte)(unhex(t, tc.b))
	user := []byte(tc.username)
	nMinus1 := nBytes()
	nMinus1[PublicSize-1]--
	var zero [SecretSize]byte
	for _, f := range []struct {
		name string
		v    []byte
		b    *[SecretSize]byte
		want error
	}{
		{name: "v = N-1", v: nMinus1, b: b},
		{name: "v = 0", v: make([]byte, PublicSize), b: b, want: ErrVerifier},
		{name: "v = N", v: nBytes(), b: b, want: ErrVerifier},
		{name: "b = 0", v: unhex(t, tc.v), b: &zero, want: ErrSecret},
	} {
		t.Run(f.name, func(t *testing.T) {
			var s Server
			if err := s.Init(user, (*[PublicSize]byte)(f.v), salt, f.b); err != f.want {
				t.Fatalf("Init: %v, want %v", err, f.want)
			}
		})
	}
	var c Client
	if err := c.Init(&zero); err != ErrSecret {
		t.Fatalf("client Init with a = 0: %v, want %v", err, ErrSecret)
	}
	var m1 [ProofSize]byte
	var k [KeySize]byte
	if err := c.Proof(user, nil, salt, nBytes(), &m1, &k); err != ErrState {
		t.Fatalf("Proof without Init: %v, want %v", err, ErrState)
	}
}

// TestRoundTrip runs exchanges with other secrets, salts and passwords.
func TestRoundTrip(t *testing.T) {
	for i := 0; i < 4; i++ {
		var salt [SaltSize]byte
		var a, b [SecretSize]byte
		for j := range a {
			a[j], b[j] = byte(31*i+7*j+1), byte(17*i+3*j+5)
		}
		for j := range salt {
			salt[j] = byte(i * j)
		}
		pass := bytes.Repeat([]byte{'0' + byte(i)}, i+1)
		var s Server
		var c Client
		if err := s.InitPassword([]byte(Username), pass, &salt, &b); err != nil {
			t.Fatal(err)
		}
		c.Init(&a)
		var m1, m2 [ProofSize]byte
		var ck, sk [KeySize]byte
		if err := c.Proof([]byte(Username), pass, s.Salt(), s.PublicKey()[:], &m1, &ck); err != nil {
			t.Fatal(err)
		}
		pa := *c.PublicKey()
		if err := s.Verify(pa[:], &m1, &m2, &sk); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if err := c.Verify(&m2); err != nil || sk != ck {
			t.Fatalf("%d: %v", i, err)
		}
	}
}

// TestServerTime logs the time of one server-side exchange: InitPassword,
// which computes v and B, and Verify.
func TestServerTime(t *testing.T) {
	tc := vectors[0]
	user, pass := []byte(tc.username), []byte(tc.password)
	salt := (*[SaltSize]byte)(unhex(t, tc.salt))
	b := (*[SecretSize]byte)(unhex(t, tc.b))
	pa := unhex(t, tc.A)
	m1 := (*[ProofSize]byte)(unhex(t, tc.M1))
	var m2 [ProofSize]byte
	var k [KeySize]byte
	const rounds = 5
	start := time.Now()
	for i := 0; i < rounds; i++ {
		var s Server
		if err := s.InitPassword(user, pass, salt, b); err != nil {
			t.Fatal(err)
		}
		if err := s.Verify(pa, m1, &m2, &k); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("server exchange: %v", time.Since(start)/rounds)
}
