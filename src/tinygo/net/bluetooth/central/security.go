package central

import (
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/smp"
)

// Bonds stores bonds, for example in flash. Bond copies entry i and reports false after the last
// entry. peer is the address that the peer used when it paired.
type Bonds interface {
	Bond(i int, peer *smp.Address, b *smp.Bond) bool
	SaveBond(peer *smp.Address, b *smp.Bond) error
}

const (
	ErrNotConnected Error = "central: no connection"
	ErrNoPairing    Error = "central: pairing is not set up, Config.RNG and Config.Bonds are nil"
)

// Pairing returns the pairing state, and Encrypted whether the link is encrypted now.
func (c *Central) Pairing() smp.State {
	if c.pairing == nil {
		return smp.StateIdle
	}
	return c.pairing.State()
}
func (c *Central) Encrypted() bool { return c.encrypted }

// SecurityRequested reports whether the peripheral asked for security on this connection.
func (c *Central) SecurityRequested() bool {
	return c.pairing != nil && c.pairing.SecurityRequested()
}

// Secure encrypts the link with the LTK of a stored bond with the peer, or pairs first when there
// is none. Encryption Change and the end of key distribution finish it.
func (c *Central) Secure() error {
	if c.conn != ConnConnected {
		return ErrNotConnected
	}
	if c.pairing == nil {
		return ErrNoPairing
	}
	if c.findBond() {
		return c.encrypt()
	}
	c.encStarted = false
	return c.pairing.Pair(c.cfg.Bonding, c.now)
}

// findBond copies the LTK of the stored bond with the peer into c.ltk.
func (c *Central) findBond() bool {
	found := false
	for i := 0; !found && c.cfg.Bonds.Bond(i, &c.foundPeer, &c.found); i++ {
		if found = smp.BondMatches(c.mem, &c.found, c.foundPeer, c.peerID); found {
			c.ltk = c.found.LTK
		}
	}
	c.found = smp.Bond{}
	return found
}

// encrypt starts encryption with c.ltk. Secure Connections keys use Rand and EDIV 0 (Vol 3 Part H 2.4.4.1).
func (c *Central) encrypt() error {
	n, err := hci.PutLEEnableEncryption(c.cmd[:], c.handle, 0, 0, &c.ltk)
	clear(c.ltk[:])
	if err != nil {
		return err
	}
	err = c.command(n)
	clear(c.cmd[:])
	c.encStarted = err == nil
	return err
}

// smpFrame gives an SMP PDU to the initiator, starts encryption after the DHKey check, and saves the bond.
func (c *Central) smpFrame(p []byte) error {
	if c.pairing == nil {
		return nil
	}
	if err := c.pairing.Handle(p, c.now); err != nil {
		return err
	}
	if c.pairing.State() == smp.StateDistributing && !c.encStarted && c.pairing.LTK(&c.ltk) {
		return c.encrypt()
	}
	return c.saveBond()
}

// encryptionChange tells the initiator about encryption, which starts key distribution.
func (c *Central) encryptionChange(ec hci.EncryptionChange) error {
	if c.conn != ConnConnected || ec.Handle != c.handle {
		return nil
	}
	c.encrypted = ec.Status == 0 && ec.Enabled
	if !c.encrypted || c.pairing == nil {
		return nil
	}
	if err := c.pairing.Encrypted(c.now); err != nil {
		return err
	}
	return c.saveBond()
}

// saveBond stores the bond once pairing ends, when both sides asked for bonding.
func (c *Central) saveBond() error {
	if c.saved || c.pairing.State() != smp.StatePaired || !c.pairing.Bond(&c.found) {
		return nil
	}
	c.saved = true
	var err error
	if c.found.Bonded {
		err = c.cfg.Bonds.SaveBond(&c.peerID, &c.found)
	}
	c.found = smp.Bond{}
	return err
}
