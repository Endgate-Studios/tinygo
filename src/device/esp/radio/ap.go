//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"

	"tinygo/net/wifi/eapol"
	"tinygo/net/wifi/ieee80211"
	"tinygo/net/wifi/wpa"
)

// The soft-AP: WPA2-PSK with CCMP, or open. It does the work of the hostapd
// part of the ESP-IDF v5.3.2 supplicant (esp_supplicant/src/esp_hostap.c and
// src/ap/wpa_auth.c) with tinygo/net/wifi/eapol.

// MaxStations is the largest number of stations of the access point.
const MaxStations = 4

// Values of esp_supplicant/src/esp_wifi_driver.h and src/common/ieee802_11_defs.h.
const (
	appieWPA             = 3 // WIFI_APPIE_WPA
	appieAssocResp       = 2 // WIFI_APPIE_ASSOC_RESP
	reasonPrevAuth       = 2 // WLAN_REASON_PREV_AUTH_NOT_VALID
	reason4WayTimeout    = 15
	eapolResendInterval  = time.Second // the eloop timer of wpa_auth.c
	eapolResendCheckTime = 100 * time.Millisecond
)

type apStation struct {
	used     bool
	mac      ieee80211.MAC
	h        *eapol.Handshake
	deadline time.Time // of the next resend, or zero
}

var ap struct {
	mu       sync.Mutex
	auth     *eapol.Authenticator
	aa       ieee80211.MAC
	sta      [MaxStations]apStation
	resender bool
	tx       [14 + 256]byte
}

// APConfig is the configuration of the access point.
type APConfig struct {
	SSID     string
	Password string // 8 to 63 characters for WPA2-PSK, or empty for an open AP
	Channel  uint8  // 1 to 13
	// MaxStations is from 1 to MaxStations. 0 means MaxStations.
	MaxStations uint8
}

// ConfigureAP sets the access point, as esp_wifi_set_config(WIFI_IF_AP), after
// Init and SetMode. The blobs derive the PMK in it with crypto.go, and the
// authenticator uses that PMK, as hostap_init of esp_hostap.c.
func ConfigureAP(c APConfig) error {
	defer releaseThreadSemaphore()
	if c.SSID == "" || len(c.SSID) > 32 || c.Password != "" && (len(c.Password) < 8 || len(c.Password) > 63) {
		return ErrInvalidArg
	}
	if c.Channel == 0 {
		c.Channel = 1
	}
	if c.MaxStations == 0 || c.MaxStations > MaxStations {
		c.MaxStations = MaxStations
	}
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.auth != nil {
		ap.auth.Close()
		ap.auth = nil
	}
	ssid, pass := []byte(c.SSID), []byte(c.Password)
	var pp *C.uint8_t
	if len(pass) > 0 {
		pp = (*C.uint8_t)(unsafe.Pointer(&pass[0]))
	}
	if err := check(C.seam_wifi_set_ap((*C.uint8_t)(unsafe.Pointer(&ssid[0])), C.uint8_t(len(ssid)), pp, C.uint8_t(len(pass)), C.uint8_t(c.Channel), C.uint8_t(c.MaxStations))); err != nil {
		return err
	}
	if len(pass) == 0 {
		return nil
	}
	ap.aa = ieee80211.MAC(AccessPoint{}.MAC())
	pmk := unsafe.Slice((*byte)(unsafe.Pointer(C.esp_wifi_ap_get_prof_pmk_internal())), wpa.PMKLen)
	rsn := unsafe.Slice((*byte)(unsafe.Pointer(&C.seam_ap_rsn[0])), len(C.seam_ap_rsn))
	a, err := eapol.NewAuthenticatorPMK(blobHeap, hwRandom{}, ap.aa, rsn, pmk)
	if err != nil {
		return err
	}
	ap.auth = a
	return nil
}

// wpaAPInit is wpa_ap_init (hostap_init): it installs the GTK and sets the RSN
// element of the beacon.
//
//export tinygo_wifi_wpa_ap_init
func wpaAPInit() bool {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.auth == nil {
		return false
	}
	gtk, id := ap.auth.GTK()
	broadcast := [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	if C.esp_wifi_set_ap_key_internal(algCCMP, (*C.uint8_t)(&broadcast[0]), C.int(id), (*C.uint8_t)(unsafe.Pointer(&gtk[0])), C.size_t(len(gtk))) != 0 {
		return false
	}
	C.esp_wifi_set_appie_internal(appieWPA, (*C.uint8_t)(unsafe.Pointer(&C.seam_ap_rsn[0])), C.uint16_t(len(C.seam_ap_rsn)), 0)
	if !ap.resender {
		ap.resender = true
		go resendLoop()
	}
	return true
}

//export tinygo_wifi_wpa_ap_deinit
func wpaAPDeinit() {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	for i := range ap.sta {
		removeLocked(i)
	}
	C.esp_wifi_unset_appie_internal(appieWPA)
	C.esp_wifi_unset_appie_internal(appieAssocResp)
}

// wpaAPJoin is wpa_ap_join (hostap_sta_join): it checks the RSN element of
// the station, sends the association response and message 1.
//
//export tinygo_wifi_wpa_ap_join
func wpaAPJoin(bssid *byte, ie *byte, ieLen uint8, subtype int32, slot *int32) bool {
	mac := ieee80211.MAC(unsafe.Slice(bssid, 6))
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.auth == nil {
		return false
	}
	i := -1
	for j := range ap.sta {
		if ap.sta[j].used && ap.sta[j].mac == mac {
			i = j // the station associates again
			break
		}
	}
	if i < 0 {
		for j := range ap.sta {
			if !ap.sta[j].used {
				i = j
				break
			}
		}
	}
	if i < 0 {
		return false
	}
	removeLocked(i)
	h, err := ap.auth.NewHandshake(mac, unsafe.Slice(ie, ieLen))
	if err != nil {
		C.esp_wifi_ap_deauth_internal((*C.uint8_t)(unsafe.Pointer(bssid)), reasonPrevAuth)
		return false
	}
	s := &ap.sta[i]
	s.used, s.mac, s.h = true, mac, h
	if C.seam_ap_send_assoc_resp((*C.uint8_t)(unsafe.Pointer(bssid)), 0, C.int(subtype)) != 0 {
		removeLocked(i)
		return false
	}
	n, err := h.Start(ap.tx[14:])
	if err != nil {
		removeLocked(i)
		return false
	}
	sendEAPOLLocked(s, n)
	*slot = int32(i)
	return true
}

//export tinygo_wifi_wpa_ap_remove
func wpaAPRemove(bssid *byte) bool {
	mac := ieee80211.MAC(unsafe.Slice(bssid, 6))
	ap.mu.Lock()
	defer ap.mu.Unlock()
	for i := range ap.sta {
		if ap.sta[i].used && ap.sta[i].mac == mac {
			removeLocked(i)
			return true
		}
	}
	return false
}

// wpaAPRxEAPOL is wpa_ap_rx_eapol: messages 2 and 4. After message 4 it
// installs the pairwise key and marks the station authorized.
//
//export tinygo_wifi_wpa_ap_rx_eapol
func wpaAPRxEAPOL(slot int32, data *byte, n uintptr) {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if slot < 0 || int(slot) >= len(ap.sta) || !ap.sta[slot].used {
		return
	}
	s := &ap.sta[slot]
	m, done, err := s.h.Handle(unsafe.Slice(data, n), ap.tx[14:])
	switch {
	case err == eapol.ErrRSNMismatch:
		C.esp_wifi_ap_deauth_internal((*C.uint8_t)(unsafe.Pointer(&s.mac[0])), reasonPrevAuth)
	case err != nil:
		// A bad MIC or an old frame: drop it, as wpa_receive of wpa_auth.c.
	case done:
		s.deadline = time.Time{}
		tk := s.h.TK()
		if C.esp_wifi_set_ap_key_internal(algCCMP, (*C.uint8_t)(unsafe.Pointer(&s.mac[0])), 0, (*C.uint8_t)(unsafe.Pointer(&tk[0])), C.size_t(len(tk))) != 0 {
			C.esp_wifi_ap_deauth_internal((*C.uint8_t)(unsafe.Pointer(&s.mac[0])), reasonPrevAuth)
			return
		}
		C.esp_wifi_wpa_ptk_init_done_internal((*C.uint8_t)(unsafe.Pointer(&s.mac[0])))
	case m > 0:
		sendEAPOLLocked(s, m)
	}
}

// resendLoop sends message 1 or 3 again after each second without an answer,
// and removes the station after the last try, as the eloop timer of wpa_auth.c.
func resendLoop() {
	for {
		time.Sleep(eapolResendCheckTime)
		now := time.Now()
		ap.mu.Lock()
		for i := range ap.sta {
			s := &ap.sta[i]
			if !s.used || s.deadline.IsZero() || now.Before(s.deadline) || !s.h.Waiting() {
				continue
			}
			n, err := s.h.Resend(ap.tx[14:])
			if err != nil {
				mac := s.mac
				removeLocked(i)
				C.esp_wifi_ap_deauth_internal((*C.uint8_t)(unsafe.Pointer(&mac[0])), reason4WayTimeout)
				continue
			}
			sendEAPOLLocked(s, n)
		}
		ap.mu.Unlock()
	}
}

// sendEAPOLLocked sends the EAPOL frame at ap.tx[14:14+n] to the station.
func sendEAPOLLocked(s *apStation, n int) {
	copy(ap.tx[0:6], s.mac[:])
	copy(ap.tx[6:12], ap.aa[:])
	ap.tx[12], ap.tx[13] = eapol.EtherType>>8, eapol.EtherType&0xff
	send(ifAP, ap.tx[:14+n])
	s.deadline = time.Now().Add(eapolResendInterval)
}

func removeLocked(i int) {
	s := &ap.sta[i]
	if s.h != nil {
		s.h.Close()
	}
	*s = apStation{}
}
