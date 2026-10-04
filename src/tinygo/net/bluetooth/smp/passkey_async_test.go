package smp

import (
	"math/rand"
	"testing"
	"time"
)

type chanSender struct{ ch chan []byte }

func (s chanSender) SendSMP(p []byte) error {
	s.ch <- append([]byte(nil), p...)
	return nil
}

// screen shows the passkey of the initiator to the user, and keyboard is where the user types it
// at the responder, with an error of typo.
type screen struct{ ch chan uint32 }

func (s screen) ShowPasskey(v uint32) { s.ch <- v }

type keyboard struct {
	ch   chan uint32
	typo uint32
}

func (k keyboard) EnterPasskey() (uint32, bool) {
	select {
	case v := <-k.ch:
		return v + k.typo, true
	case <-time.After(5 * time.Second):
		return 0, false
	}
}

// runSide handles PDUs until the side has the LTK or fails, then reports its state. Only this
// goroutine touches the side and its memory until then.
func runSide(handle func([]byte, uint32) error, state func() State, in chan []byte, done chan<- State) {
	for {
		select {
		case p := <-in:
			if err := handle(p, 0); err != nil {
				done <- StateIdle
				return
			}
			if st := state(); st == StateDistributing || st == StatePaired || st == StateFailed {
				done <- st
				return
			}
		case <-time.After(5 * time.Second):
			done <- StateIdle
			return
		}
	}
}

func pairAsync(t *testing.T, typo uint32) (*Initiator, *Responder, State, State, *checkedMemory, *checkedMemory) {
	t.Helper()
	toI, toR := make(chan []byte, 64), make(chan []byte, 64)
	user := make(chan uint32, 1)
	imem, rmem := newCheckedMemory(t), newCheckedMemory(t)
	// The initiator has a display and the responder only a keyboard, so the initiator shows.
	i, _ := NewInitiator(imem, rand.New(rand.NewSource(41)), chanSender{toR}, nil, IODisplayOnly)
	r, _ := NewResponder(rmem, rand.New(rand.NewSource(43)), chanSender{toI}, nil, IOKeyboardOnly)
	i.SetPasskeyIO(screen{user}, nil)
	r.SetPasskeyIO(nil, keyboard{ch: user, typo: typo})
	i.Start(central, peripheral)
	r.Start(peripheral, central)
	if err := i.Pair(false, 0); err != nil {
		t.Fatal(err)
	}
	doneI, doneR := make(chan State, 1), make(chan State, 1)
	go runSide(i.Handle, i.State, toI, doneI)
	go runSide(r.Handle, r.State, toR, doneR)
	return i, r, <-doneI, <-doneR, imem, rmem
}

func TestPasskeyInitiatorShows(t *testing.T) {
	i, r, si, sr, imem, rmem := pairAsync(t, 0)
	// Without keys to distribute, the responder is paired at once.
	if si != StateDistributing || sr != StatePaired {
		t.Fatalf("states %d and %d, errors %v and %v", si, sr, i.Err(), r.Err())
	}
	var li, lr [16]byte
	if !i.LTK(&li) || !r.LTK(&lr) || li != lr {
		t.Fatalf("LTK % x and % x", li, lr)
	}
	// No keys are asked without bonding, so encryption ends pairing on both sides.
	r.Encrypted(0)
	i.Encrypted(0)
	if i.State() != StatePaired || r.State() != StatePaired {
		t.Errorf("after encryption: states %d and %d", i.State(), r.State())
	}
	i.Close()
	r.Close()
	imem.check(t)
	rmem.check(t)
}

func TestPasskeyInitiatorShowsTypo(t *testing.T) {
	i, r, si, sr, imem, rmem := pairAsync(t, 1)
	if si != StateFailed || sr != StateFailed {
		t.Errorf("typo: states %d and %d", si, sr)
	}
	i.Close()
	r.Close()
	imem.check(t)
	rmem.check(t)
}
