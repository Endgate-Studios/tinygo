//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"tinygo/net/wifi/eapol"
	"tinygo/net/wifi/ieee80211"
	"unsafe"
)

// The station supplicant: WPA2-PSK with CCMP, and open networks. It does the
// work of esp_wpa_main.c and rsn_supp/wpa.c of the ESP-IDF v5.3.2 supplicant
// with tinygo/net/wifi/eapol.

// Values of ESP-IDF v5.3.2 esp_supplicant/src/esp_wifi_driver.h.
const (
	authNone     = 0x01 // NONE_AUTH
	appieRSN     = 4    // WIFI_APPIE_RSN
	appieAssoc   = 1    // WIFI_APPIE_ASSOC_REQ
	algCCMP      = 3    // WIFI_WPA_ALG_CCMP
	keyFlagRX    = 1 << 2
	keyFlagTX    = 1 << 3
	keyFlagGroup = 1 << 4
	keyFlagPair  = 1 << 5
	cipherCCMP   = 3 // the bit index of WPA_CIPHER_CCMP
)

// assocIELen is ASSOC_IE_LEN - 2 of rsn_supp/wpa.c for RSN.
const assocIELen = 44

// RSN capabilities of wpa_supplicant/src/common/wpa_common.h.
const rsnCapSPP = 1<<10 | 1<<11

type supState uint8

const (
	supIdle supState = iota
	supAssoc
	supFirstHalf // after message 1
	supLastHalf  // after message 3, message 4 is in the TX queue
	supDone
)

var sup struct {
	s          *eapol.Supplicant
	ssid, pass string
	state      supState
	aa, spa    ieee80211.MAC
	reply      [256]byte
	tx         [14 + 256]byte
}

// assocIE is the RSN element of the association request. The blobs keep a
// pointer to it (flag 1). The element starts at offset 2.
var assocIE [2 + assocIELen]byte

// hwRandom reads the hardware random number generator.
type hwRandom struct{}

func (hwRandom) Read(b []byte) (int, error) {
	for i := 0; i < len(b); i += 4 {
		r := random()
		for j := i; j < i+4 && j < len(b); j++ {
			b[j] = byte(r)
			r >>= 8
		}
	}
	return len(b), nil
}

// prepareSupplicant derives the PMK before the connection, so the 4096
// rounds of PBKDF2 do not run in a callback of the blobs.
func prepareSupplicant(ssid, password string) error {
	if password == "" || (sup.s != nil && sup.ssid == ssid && sup.pass == password) {
		return nil
	}
	if sup.s != nil {
		sup.s.Close()
		sup.s = nil
	}
	var s *eapol.Supplicant
	var err error
	if psk, ok := hexPSK(password); ok {
		s, err = eapol.NewPSK(blobHeap, hwRandom{}, psk[:])
		clear(psk[:])
	} else {
		s, err = eapol.New(blobHeap, hwRandom{}, []byte(password), []byte(ssid))
	}
	if err != nil {
		return err
	}
	sup.s, sup.ssid, sup.pass = s, ssid, password
	return nil
}

// hexPSK decodes a password of 64 hex digits, which is the PSK itself, as
// wpa_set_passphrase of rsn_supp/wpa.c.
func hexPSK(password string) (psk [32]byte, ok bool) {
	if len(password) != 2*len(psk) {
		return psk, false
	}
	for i := range psk {
		hi, lo := hexDigit(password[2*i]), hexDigit(password[2*i+1])
		if hi < 0 || lo < 0 {
			return psk, false
		}
		psk[i] = byte(hi<<4 | lo)
	}
	return psk, true
}

//export tinygo_wifi_wpa_sta_deinit
func wpaStaDeinit() {
	sup.state = supIdle
	if sup.s != nil {
		sup.s.Close()
		sup.s, sup.ssid, sup.pass = nil, "", ""
	}
}

// wpaStaConnect is wpa_sta_connect. It sets the RSN element and starts the
// association.
//
//export tinygo_wifi_wpa_sta_connect
func wpaStaConnect(bssid *byte) int32 {
	sup.state = supIdle
	if !C.esp_wifi_sta_prof_is_rsn_internal() {
		if C.esp_wifi_sta_get_prof_authmode_internal() != authNone {
			println("wifi: only open and WPA2 networks are supported")
			return -1
		}
		setEmptyAssocIE()
		return int32(C.esp_wifi_sta_connect_internal((*C.uint8_t)(bssid)))
	}
	pairwise := C.esp_wifi_sta_get_pairwise_cipher_internal()
	group := C.esp_wifi_sta_get_group_cipher_internal()
	if pairwise != cipherCCMP || group != cipherCCMP || sup.s == nil {
		println("wifi: only WPA2-PSK with CCMP is supported")
		return -1
	}
	copy(sup.aa[:], unsafe.Slice(bssid, 6))
	C.esp_wifi_get_macaddr_internal(ifStation, (*C.uint8_t)(unsafe.Pointer(&sup.spa[0])))

	// set_assoc_ie gives the buffer to the blobs before wpa_gen_wpa_ie
	// writes the element into it.
	C.esp_wifi_set_appie_internal(appieRSN, (*C.uint8_t)(unsafe.Pointer(&assocIE[0])), assocIELen, 1)
	ie := genRSN(assocIE[2:])
	setEmptyAssocIE()
	if err := sup.s.Start(sup.aa, sup.spa, ie, nil); err != nil {
		println("wifi: supplicant:", err.Error())
		return -1
	}
	sup.state = supAssoc
	return int32(C.esp_wifi_sta_connect_internal((*C.uint8_t)(bssid)))
}

// genRSN is wpa_gen_wpa_ie_rsn of rsn_supp/wpa_ie.c for PSK and CCMP
// without PMF. Some blobs want only the group suite.
func genRSN(b []byte) []byte {
	ie := append(b[:0], 48, 0, 1, 0, 0x00, 0x0f, 0xac, 4)
	if C.esp_wifi_sta_is_ap_notify_completed_rsne_internal() {
		capab := uint16(C.esp_wifi_get_spp_attrubute_internal(ifStation)) & rsnCapSPP
		ie = append(ie,
			1, 0, 0x00, 0x0f, 0xac, 4, // pairwise CCMP
			1, 0, 0x00, 0x0f, 0xac, 2, // AKM PSK
			byte(capab), byte(capab>>8))
	}
	ie[1] = byte(len(ie) - 2)
	return ie
}

// setEmptyAssocIE is esp_set_assoc_ie without extra elements.
func setEmptyAssocIE() {
	var empty [1]byte
	C.esp_wifi_set_appie_internal(appieAssoc, (*C.uint8_t)(unsafe.Pointer(&empty[0])), 0, 0)
}

//export tinygo_wifi_wpa_sta_disconnected
func wpaStaDisconnected(reason uint8) {
	sup.state = supIdle
}

//export tinygo_wifi_wpa_sta_in_4way_handshake
func wpaStaIn4WayHandshake() bool {
	return sup.state == supFirstHalf || sup.state == supLastHalf
}

// wpaStaRxEAPOL is wpa_sm_rx_eapol. buf starts at the 802.1X header.
//
//export tinygo_wifi_wpa_sta_rx_eapol
func wpaStaRxEAPOL(src, buf *byte, n uint32) int32 {
	if sup.s == nil || sup.state == supIdle {
		return -1
	}
	frame := unsafe.Slice(buf, n)
	m, ev, err := sup.s.Handle(frame, sup.reply[:])
	if err != nil {
		println("wifi: EAPOL:", err.Error())
		return -1
	}
	switch {
	case ev == eapol.EventPairwise:
		sup.state = supLastHalf
	case ev == eapol.EventGroup:
		// process_1_of_2 sets the new group key before it sends message 2.
		installGTK()
	case sup.state == supAssoc:
		sup.state = supFirstHalf
	}
	if m > 0 {
		sendEAPOL(sup.reply[:m])
	}
	return 0
}

// sendEAPOL is wpa_ether_send: an Ethernet header and the EAPOL frame.
func sendEAPOL(frame []byte) {
	b := sup.tx[:14+len(frame)]
	copy(b[0:6], sup.aa[:])
	copy(b[6:12], sup.spa[:])
	b[12], b[13] = 0x88, 0x8e // ETH_P_EAPOL
	copy(b[14:], frame)
	C.esp_wifi_internal_tx(ifStation, unsafe.Pointer(&b[0]), C.uint16_t(len(b)))
}

// eapolTxDone is eapol_txcb. After message 4 is sent, it sets the keys and
// tells the blobs that the handshake is done
// (wpa_supplicant_send_4_of_4_txcallback).
//
//export tinygo_wifi_eapol_txdone
func eapolTxDone(payload *byte, n uintptr, failure bool) {
	if sup.state != supLastHalf || failure || n < 4+5 {
		return
	}
	p := unsafe.Slice(payload, n)
	const keyInfoSecure = 1 << 9
	if p[1] != 3 || (uint16(p[5])<<8|uint16(p[6]))&keyInfoSecure == 0 {
		return // not message 4
	}
	var seq [8]byte
	tk := sup.s.TK()
	C.esp_wifi_set_sta_key_internal(algCCMP, (*C.uint8_t)(unsafe.Pointer(&sup.aa[0])), 0, 1,
		(*C.uint8_t)(unsafe.Pointer(&seq[0])), C.size_t(len(seq)),
		(*C.uint8_t)(unsafe.Pointer(&tk[0])), C.size_t(len(tk)), keyFlagPair|keyFlagTX|keyFlagRX)
	installGTK()
	sup.state = supDone
	C.esp_wifi_auth_done_internal()
}

// installGTK is wpa_supplicant_install_gtk. The sequence is the 6-byte CCMP
// packet number of the key RSC.
func installGTK() {
	gtk, id := sup.s.GTK()
	if gtk == nil {
		return
	}
	seq := sup.s.GTKRSC()
	C.esp_wifi_set_sta_key_internal(algCCMP, (*C.uint8_t)(unsafe.Pointer(&sup.aa[0])), C.int(id), 0,
		(*C.uint8_t)(unsafe.Pointer(&seq[0])), 6,
		(*C.uint8_t)(unsafe.Pointer(&gtk[0])), C.size_t(len(gtk)), keyFlagGroup|keyFlagRX)
}
