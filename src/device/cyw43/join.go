package cyw43

import (
	"time"
)

// Firmware events (brcmfmac fweh.h, BRCMF_E_*).
const (
	eventSetSSID   = 0
	eventJoin      = 1
	eventAuth      = 3
	eventDeauth    = 5
	eventDisassoc  = 11
	eventLink      = 16
	eventPSKSup    = 46
	eventDeauthInd = 6
	eventDisassocI = 12

	supKeyed        = 6 // WLC_SUP_KEYED
	etherTypeBRCM   = 0x886c
	eventHeaderSkip = 14 + 10 // Ethernet header, then struct brcm_ethhdr
)

// eventState is what the join waits on, set from received events.
type eventState struct {
	setSSID   bool
	ssidOK    bool
	link      bool
	keyed     bool
	authFail  bool
	lost      bool
	lastEvent uint32
}

// handle reads a struct brcmf_event_msg_be after the Ethernet and BRCM headers.
func (e *eventState) handle(p []byte) {
	if len(p) < eventHeaderSkip+48 || be16(p[12:]) != etherTypeBRCM {
		return
	}
	m := p[eventHeaderSkip:]
	flags := be16(m[2:])
	typ := be32(m[4:])
	status := be32(m[8:])
	e.lastEvent = typ
	switch typ {
	case eventSetSSID:
		e.setSSID = true
		e.ssidOK = status == 0
	case eventLink:
		e.link = flags&1 != 0
		if !e.link {
			e.lost = true
		}
	case eventPSKSup:
		if status == supKeyed {
			e.keyed = true
		} else {
			e.authFail = true
		}
	case eventAuth:
		if status != 0 {
			e.authFail = true
		}
	case eventDeauth, eventDeauthInd, eventDisassoc, eventDisassocI:
		e.lost = true
		e.link = false
	}
}

// Join joins the network ssid. An empty passphrase is an open network, else WPA2-PSK with
// the chip's supplicant, as embassy-rs cyw43 join_wpa2. It waits up to timeout.
func (d *Device) Join(ssid, passphrase string, timeout time.Duration) error {
	if len(ssid) > 32 || len(passphrase) > 64 {
		return ErrJoin
	}
	d.mu.Lock()
	d.events = eventState{}
	d.joined = false
	d.mu.Unlock()

	if passphrase == "" {
		d.setU32(cmdSetWsec, 0)
		d.setBssVarU32("sup_wpa", 0)
		d.setU32(cmdSetWPA, 0)
	} else {
		d.setU32(cmdSetWsec, 4) // AES_ENABLED
		d.setBssVarU32("sup_wpa", 1)
		d.setBssVarU32("sup_wpa2_eapver", 0xffffffff)
		d.setBssVarU32("sup_wpa_tmo", 2500)
		time.Sleep(100 * time.Millisecond)

		// struct brcmf_wsec_pmk_le: key length, flags (BRCMF_WSEC_PASSPHRASE), key.
		var pmk [2 + 2 + 64]byte
		put16(pmk[0:], uint16(len(passphrase)))
		put16(pmk[2:], 1)
		copy(pmk[4:], passphrase)
		if _, err := d.ioctl(cmdSetPMK, true, pmk[:], nil); err != nil {
			return err
		}
	}
	if err := d.setU32(cmdSetInfra, 1); err != nil {
		return err
	}
	d.setU32(cmdSetAuth, 0)
	if passphrase != "" {
		d.setU32(cmdSetWPA, 0x80) // WPA2_AUTH_PSK
	}

	var req [4 + 32]byte
	put32(req[0:], uint32(len(ssid)))
	copy(req[4:], ssid)
	if _, err := d.ioctl(cmdSetSSID, true, req[:], nil); err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		d.receive()
		e := d.events
		d.mu.Unlock()
		if e.authFail || (e.setSSID && !e.ssidOK) {
			return ErrJoin
		}
		if e.link && (passphrase == "" || e.keyed) {
			d.joined = true
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return ErrTimeout
}

// Joined says whether the station is joined and its link is up.
func (d *Device) Joined() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.joined && !d.events.lost
}

// Leave leaves the network.
func (d *Device) Leave() error {
	d.joined = false
	_, err := d.ioctl(cmdDisassoc, true, nil, nil)
	return err
}
