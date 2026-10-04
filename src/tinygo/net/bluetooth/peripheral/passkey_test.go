package peripheral

import (
	"math/rand"
	"testing"

	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

type passkeyScreen struct{ shown []uint32 }

func (s *passkeyScreen) ShowPasskey(v uint32) { s.shown = append(s.shown, v) }

// The Config passkey display reaches the pairing responder: a central with only a keyboard gets a
// passkey pairing, and the peripheral shows the passkey after the public keys are exchanged.
func TestPasskeyDisplayWired(t *testing.T) {
	screen := &passkeyScreen{}
	h := newHarnessWith(t, &store{}, func(c *Config) {
		c.IOCap = smp.IODisplayOnly
		c.PasskeyDisplay = screen
	})
	h.start()
	h.connect(smp.Address{Type: 1, Addr: [6]byte{1, 2, 3, 4, 5, 0xc6}})
	h.send(l2cap.CIDSMP, []byte{0x01, smp.IOKeyboardOnly, 0, 0x0d, 16, 0, 0})
	if pres := h.pop(l2cap.CIDSMP, 0x02); pres[3]&0x04 == 0 {
		t.Fatalf("Pairing Response % x has no MITM flag", pres)
	}
	ka, _ := ecdhKey(rand.New(rand.NewSource(2)))
	pka := ka.PublicKey().Bytes()
	h.send(l2cap.CIDSMP, cat([]byte{0x0c}, rev(pka[1:33]), rev(pka[33:])))
	h.pop(l2cap.CIDSMP, 0x0c)
	if len(screen.shown) != 1 || screen.shown[0] > 999999 {
		t.Errorf("shown passkeys %v, want one value up to 999999", screen.shown)
	}
	if len(h.frames) != 0 {
		t.Errorf("the peripheral sent % x before the first confirm value of the central", h.frames)
	}
}

// Without a display, the same request fails with Pairing Not Supported.
func TestPasskeyWithoutDisplay(t *testing.T) {
	h := newHarnessWith(t, &store{}, func(c *Config) { c.IOCap = smp.IODisplayOnly })
	h.start()
	h.connect(smp.Address{Type: 1, Addr: [6]byte{1, 2, 3, 4, 5, 0xc6}})
	h.send(l2cap.CIDSMP, []byte{0x01, smp.IOKeyboardOnly, 0, 0x0d, 16, 0, 0})
	if f := h.pop(l2cap.CIDSMP, 0x05); f[1] != smp.ReasonPairingNotSupported {
		t.Errorf("Pairing Failed % x", f)
	}
}
