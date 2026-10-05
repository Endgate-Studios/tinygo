// Package sae is the Simultaneous Authentication of Equals of WPA3-Personal
// (IEEE 802.11-2020 12.4) with ECC group 19 (P-256) and the hunting and
// pecking method for the password element. Key material is in memory from an
// injected allocator and is zeroed before it is freed.
package sae

import (
	"bytes"
	"crypto/subtle"
	"io"

	"runtime/memory"
	"tinygo/crypto/p256"
	"tinygo/crypto/sha256"
	"tinygo/net/wifi/wpa"
)

// Memory is where SAE allocates its state and the working points.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Error is a string error type, so error values are constants in flash.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilMemory Error = "sae: memory cannot be nil"
	ErrNoMemory  Error = "sae: out of memory"
	ErrRandom    Error = "sae: random source failed"
	ErrPassword  Error = "sae: no password element"
	ErrGroup     Error = "sae: group is not 19"
	ErrCommit    Error = "sae: bad commit message"
	ErrReflected Error = "sae: the peer sent our own commit"
	ErrConfirm   Error = "sae: confirm does not match"
	ErrState     Error = "sae: no commit of the peer yet"
	ErrShort     Error = "sae: buffer too short"
)

const (
	// Group is the IANA group number of P-256.
	Group = 19
	// CommitLen is the length of a commit without an anti-clogging token:
	// the group, the scalar and the element.
	CommitLen = 2 + 32 + 64
	// ConfirmLen is the length of a confirm: send-confirm and the HMAC.
	ConfirmLen = 2 + sha256.Size
	// huntAndPeckRounds is k of 12.4.4.2.2. All rounds run, found or not.
	huntAndPeckRounds = 40
)

// prime is p of P-256, the context of the KDF in hunting and pecking.
var prime = [32]byte{
	0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}

type state struct {
	pweX, pweY             [32]byte
	rand                   [32]byte
	scalar, elemX, elemY   [32]byte
	peerScalar, peerX      [32]byte
	peerY                  [32]byte
	kck, pmk               [32]byte
	pmkid                  [16]byte
	committed, peerCommits bool
}

// SAE is one SAE exchange with one peer.
type SAE struct {
	mem Memory
	s   *state
}

// New derives the password element for the two MAC addresses and makes the
// commit scalar and element. rng gives rand and mask.
func New(mem Memory, rng io.Reader, own, peer [6]byte, password []byte) (*SAE, error) {
	if mem == nil {
		return nil, ErrNilMemory
	}
	s := memory.New[state](mem)
	if s == nil {
		return nil, ErrNoMemory
	}
	a := &SAE{mem: mem, s: s}
	if !derivePWE(s, own, peer, password) {
		a.Close()
		return nil, ErrPassword
	}
	var mask [32]byte
	var err error = ErrRandom
	for tries := 0; tries < 64; tries++ {
		if randomScalar(rng, &s.rand) != nil || randomScalar(rng, &mask) != nil {
			break
		}
		if err = a.commit(&mask); err == nil || err != ErrRandom {
			break
		}
	}
	clear(mask[:])
	if err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

// randomScalar draws a value in 2 to r-1.
func randomScalar(rng io.Reader, k *[32]byte) error {
	for tries := 0; tries < 64; tries++ {
		if _, err := io.ReadFull(rng, k[:]); err != nil {
			return ErrRandom
		}
		if p256.ValidScalar(k) && !isOne(k) {
			return nil
		}
	}
	return ErrRandom
}

func isOne(k *[32]byte) bool {
	var acc byte
	for _, b := range k[:31] {
		acc |= b
	}
	return acc == 0 && k[31] == 1
}

// commit computes the commit scalar (rand + mask) mod r and the commit
// element -(mask * PWE) from s.rand and mask.
func (a *SAE) commit(mask *[32]byte) error {
	s := a.s
	p256.AddModN(&s.rand, mask, &s.scalar)
	if !p256.ValidScalar(&s.scalar) || isOne(&s.scalar) {
		return ErrRandom // draw again, as sae_derive_commit of hostapd
	}
	var y [32]byte
	if err := p256.ScalarMult(a.mem, mask, &s.pweX, &s.pweY, &s.elemX, &y); err != nil {
		return err
	}
	p256.NegateY(&y, &s.elemY)
	s.committed = true
	return nil
}

// derivePWE is the hunting and pecking of 12.4.4.2.2 for an ECC group.
func derivePWE(s *state, own, peer [6]byte, password []byte) bool {
	var addrs [12]byte
	if bytes.Compare(own[:], peer[:]) > 0 {
		copy(addrs[:6], own[:])
		copy(addrs[6:], peer[:])
	} else {
		copy(addrs[:6], peer[:])
		copy(addrs[6:], own[:])
	}
	var h sha256.HMAC
	h.Init(addrs[:])
	var seed, saved [32]byte
	var value, ys [32]byte
	found := 0
	for counter := 1; counter <= huntAndPeckRounds; counter++ {
		c := [1]byte{byte(counter)}
		h.Sum(&seed, password, c[:], nil, nil)
		wpa.KDFSHA256(seed[:], "SAE Hunting and Pecking", prime[:], value[:])
		ok := 0
		if p256.YSquared(&value, &ys) && p256.IsSquare(&ys) {
			ok = 1
		}
		// Keep the first good value, without a branch on found.
		take := ok &^ found
		subtle.ConstantTimeCopy(take, s.pweX[:], value[:])
		subtle.ConstantTimeCopy(take, saved[:], seed[:])
		found |= ok
	}
	h.Clear()
	clear(seed[:])
	clear(value[:])
	if found == 0 {
		clear(saved[:])
		return false
	}
	var y [32]byte
	p256.YSquared(&s.pweX, &ys)
	p256.Sqrt(&ys, &y)
	// The parity of y follows the last bit of the saved seed.
	var neg [32]byte
	p256.NegateY(&y, &neg)
	same := subtle.ConstantTimeByteEq(saved[31]&1, y[31]&1)
	copy(s.pweY[:], neg[:])
	subtle.ConstantTimeCopy(same, s.pweY[:], y[:])
	clear(saved[:])
	clear(ys[:])
	clear(y[:])
	clear(neg[:])
	return true
}

// WriteCommit writes the commit fields after the authentication header: the
// group, token when it is not nil, the scalar and the element.
func (a *SAE) WriteCommit(b, token []byte) (int, error) {
	n := CommitLen + len(token)
	if len(b) < n {
		return 0, ErrShort
	}
	s := a.s
	b[0], b[1] = Group, 0
	o := 2 + copy(b[2:], token)
	o += copy(b[o:], s.scalar[:])
	o += copy(b[o:], s.elemX[:])
	copy(b[o:], s.elemY[:])
	return n, nil
}

// ProcessCommit checks the commit fields of the peer, without a token, and
// derives the KCK, the PMK and the PMKID.
func (a *SAE) ProcessCommit(b []byte) error {
	s := a.s
	if len(b) < 2 || int(b[0])|int(b[1])<<8 != Group {
		return ErrGroup
	}
	if len(b) != CommitLen {
		return ErrCommit
	}
	copy(s.peerScalar[:], b[2:34])
	copy(s.peerX[:], b[34:66])
	copy(s.peerY[:], b[66:98])
	if !p256.ValidScalar(&s.peerScalar) || isOne(&s.peerScalar) || !p256.OnCurve(&s.peerX, &s.peerY) {
		return ErrCommit
	}
	if s.peerScalar == s.scalar && s.peerX == s.elemX && s.peerY == s.elemY {
		return ErrReflected
	}
	// K = rand * (peer scalar * PWE + peer element), and k is its x.
	var tx, ty, k [32]byte
	err := p256.ScalarMult(a.mem, &s.peerScalar, &s.pweX, &s.pweY, &tx, &ty)
	if err == nil {
		err = p256.Add(&tx, &ty, &s.peerX, &s.peerY, &tx, &ty)
	}
	if err == nil {
		err = p256.ScalarMult(a.mem, &s.rand, &tx, &ty, &k, &ty)
	}
	clear(tx[:])
	clear(ty[:])
	if err != nil {
		clear(k[:])
		return ErrCommit
	}
	// keyseed = H(<0>32, k), and KCK || PMK = KDF-512(keyseed, "SAE KCK and
	// PMK", (scalar + peer scalar) mod r).
	var zero, keyseed, sum [32]byte
	var h sha256.HMAC
	h.Init(zero[:])
	h.Sum(&keyseed, k[:], nil, nil, nil)
	h.Clear()
	p256.AddModN(&s.scalar, &s.peerScalar, &sum)
	var keys [64]byte
	wpa.KDFSHA256(keyseed[:], "SAE KCK and PMK", sum[:], keys[:])
	copy(s.kck[:], keys[:32])
	copy(s.pmk[:], keys[32:])
	copy(s.pmkid[:], sum[:16])
	clear(k[:])
	clear(keyseed[:])
	clear(keys[:])
	clear(sum[:])
	s.peerCommits = true
	return nil
}

// confirm computes HMAC(KCK, send-confirm || scalar1 || element1 || scalar2
// || element2).
func (a *SAE) confirm(out *[sha256.Size]byte, send [2]byte, sc1, x1, y1, sc2, x2, y2 *[32]byte) {
	var h sha256.HMAC
	h.Init(a.s.kck[:])
	var first [2 + 96]byte
	copy(first[:2], send[:])
	copy(first[2:], sc1[:])
	copy(first[34:], x1[:])
	copy(first[66:], y1[:])
	h.Sum(out, first[:], sc2[:], x2[:], y2[:])
	h.Clear()
	clear(first[:])
}

// WriteConfirm writes the confirm fields with the send-confirm counter.
func (a *SAE) WriteConfirm(sendConfirm uint16, b []byte) (int, error) {
	s := a.s
	if !s.peerCommits {
		return 0, ErrState
	}
	if len(b) < ConfirmLen {
		return 0, ErrShort
	}
	send := [2]byte{byte(sendConfirm), byte(sendConfirm >> 8)}
	var c [sha256.Size]byte
	a.confirm(&c, send, &s.scalar, &s.elemX, &s.elemY, &s.peerScalar, &s.peerX, &s.peerY)
	copy(b, send[:])
	copy(b[2:], c[:])
	return ConfirmLen, nil
}

// VerifyConfirm checks the confirm fields of the peer.
func (a *SAE) VerifyConfirm(b []byte) error {
	s := a.s
	if !s.peerCommits {
		return ErrState
	}
	if len(b) != ConfirmLen {
		return ErrConfirm
	}
	var c [sha256.Size]byte
	a.confirm(&c, [2]byte{b[0], b[1]}, &s.peerScalar, &s.peerX, &s.peerY, &s.scalar, &s.elemX, &s.elemY)
	ok := subtle.ConstantTimeCompare(c[:], b[2:]) == 1
	clear(c[:])
	if !ok {
		return ErrConfirm
	}
	return nil
}

// PMK gives the pairwise master key after ProcessCommit. It is valid until
// Close.
func (a *SAE) PMK() []byte { return a.s.pmk[:] }

// PMKID gives the PMKID after ProcessCommit. It is valid until Close.
func (a *SAE) PMKID() []byte { return a.s.pmkid[:] }

// Close zeroes and frees the state.
func (a *SAE) Close() {
	if a.s == nil {
		return
	}
	*a.s = state{}
	memory.Free(a.mem, a.s)
	a.s = nil
}
