//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"runtime/memory"
)

// ESP-NOW sends short messages between Espressif chips in Wi-Fi vendor action
// frames, with no access point and no IP. The protocol is in libespnow.a. See
// ESP-IDF v5.3.2 components/esp_wifi/include/esp_now.h.

// Limits of esp_now.h for ESP-NOW v1. The encrypted peers are
// espnow_max_encrypt_num of seam.c, the ESP_WIFI_ESPNOW_MAX_ENCRYPT_NUM default
// of esp_wifi/Kconfig.
const (
	ESPNowMaxData           = 250
	ESPNowMaxPeers          = 20
	ESPNowMaxEncryptedPeers = 7
	ESPNowKeyLen            = 16
)

// Errors of esp_now.h, from ESP_ERR_ESPNOW_BASE (ESP_ERR_WIFI_BASE + 100).
const (
	ErrESPNowNotInit  Error = 0x3065
	ErrESPNowArg      Error = 0x3066
	ErrESPNowNoMem    Error = 0x3067
	ErrESPNowFull     Error = 0x3068
	ErrESPNowNotFound Error = 0x3069
	ErrESPNowInternal Error = 0x306a
	ErrESPNowExist    Error = 0x306b
	ErrESPNowIf       Error = 0x306c
	ErrESPNowChannel  Error = 0x306d
)

var (
	// ErrESPNowNoAck tells that the peer did not acknowledge a unicast
	// message, or that the blobs could not send it.
	ErrESPNowNoAck = errors.New("espnow: the peer did not acknowledge the message")
	// ErrESPNowTimeout tells that the blobs did not report the send.
	ErrESPNowTimeout = errors.New("espnow: no send status")
)

// ESPNowBroadcast is the address of all ESP-NOW devices on the channel. Add it
// as a peer before you send to it. Broadcast messages are not encrypted.
var ESPNowBroadcast = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// espnowPool is the number of received messages that wait for the reader.
const espnowPool = 8

// ESPNowPeer is a device that this device sends to.
type ESPNowPeer struct {
	MAC [6]byte
	// Channel is the Wi-Fi channel of the peer. 0 is the current channel.
	Channel uint8
	// Key is the local master key of 16 bytes for encryption, or nil.
	Key []byte
	// AccessPoint sends from the access point interface, not the station.
	AccessPoint bool
}

// ESPNowMessage is a received message. It is valid only during the call of
// the handler of HandleESPNow.
type ESPNowMessage struct {
	Src, Dst [6]byte
	RSSI     int8
	Data     []byte
}

type espnowBuf struct {
	src, dst [6]byte
	rssi     int8
	n        int
	data     [ESPNowMaxData]byte
}

var espnow struct {
	on      bool
	sendMu  sync.Mutex
	sent    chan bool
	msgs    chan espnowMsg
	free    chan *espnowBuf
	pool    unsafe.Pointer
	gen     atomic.Uint32 // changes when ESPNowDeinit frees the pool
	dropped atomic.Uint32
}

type espnowMsg struct {
	b   *espnowBuf
	gen uint32
}

// ESPNowInit starts ESP-NOW, as esp_now_init. Call it after Start. Without a
// connection, SetChannel sets the channel of all devices.
func ESPNowInit() error {
	defer releaseThreadSemaphore()
	if espnow.on {
		return nil
	}
	espnowChannels()
	bufs := memory.MakeSlice[espnowBuf](blobHeap, espnowPool)
	if bufs == nil {
		return ErrNoMem
	}
	if err := check(C.seam_espnow_init()); err != nil {
		memory.FreeSlice(blobHeap, bufs)
		return err
	}
	espnow.pool = unsafe.Pointer(&bufs[0])
	for i := range bufs {
		espnow.free <- &bufs[i]
	}
	espnow.on = true
	return nil
}

// espnowChannels makes the channels once. They stay after ESPNowDeinit.
func espnowChannels() {
	if espnow.msgs == nil {
		espnow.sent = make(chan bool, 1)
		espnow.msgs = make(chan espnowMsg, espnowPool)
		espnow.free = make(chan *espnowBuf, espnowPool)
	}
}

// ESPNowDeinit stops ESP-NOW and removes all peers. Deinit calls it.
func ESPNowDeinit() error {
	defer releaseThreadSemaphore()
	if !espnow.on {
		return nil
	}
	if err := check(C.esp_now_deinit()); err != nil {
		return err
	}
	espnow.on = false
	espnow.gen.Add(1)
	for len(espnow.free) > 0 {
		<-espnow.free
	}
	for len(espnow.msgs) > 0 {
		<-espnow.msgs
	}
	blobHeap.Free(espnow.pool)
	espnow.pool = nil
	return nil
}

// ESPNowSetPMK sets the primary master key of 16 bytes, which encrypts the
// local master keys of the peers.
func ESPNowSetPMK(pmk []byte) error {
	defer releaseThreadSemaphore()
	if len(pmk) != ESPNowKeyLen {
		return ErrInvalidArg
	}
	return check(C.esp_now_set_pmk((*C.uint8_t)(unsafe.Pointer(&pmk[0]))))
}

// ESPNowAddPeer adds a peer. A peer that exists gets the new values.
func ESPNowAddPeer(p ESPNowPeer) error {
	defer releaseThreadSemaphore()
	var key *C.uint8_t
	if p.Key != nil {
		if len(p.Key) != ESPNowKeyLen || p.MAC == ESPNowBroadcast {
			return ErrInvalidArg
		}
		key = (*C.uint8_t)(unsafe.Pointer(&p.Key[0]))
	}
	ifx := C.int(ifStation)
	if p.AccessPoint {
		ifx = ifAP
	}
	mac := (*C.uint8_t)(unsafe.Pointer(&p.MAC[0]))
	err := check(C.seam_espnow_peer(mac, key, C.uint8_t(p.Channel), ifx, false))
	if err == ErrESPNowExist {
		err = check(C.seam_espnow_peer(mac, key, C.uint8_t(p.Channel), ifx, true))
	}
	return err
}

// ESPNowRemovePeer removes a peer.
func ESPNowRemovePeer(mac [6]byte) error {
	defer releaseThreadSemaphore()
	return check(C.esp_now_del_peer((*C.uint8_t)(unsafe.Pointer(&mac[0]))))
}

// ESPNowPeerInfo is a peer in the list of ESPNowPeers.
type ESPNowPeerInfo struct {
	MAC         [6]byte
	Channel     uint8
	Encrypted   bool
	AccessPoint bool
}

// ESPNowPeers fills peers from the peer list and gives the number of peers
// that it filled. A list of ESPNowMaxPeers holds all peers. As
// esp_now_fetch_peer, it skips the broadcast and multicast peers.
func ESPNowPeers(peers []ESPNowPeerInfo) (int, error) {
	defer releaseThreadSemaphore()
	n := 0
	for n < len(peers) {
		p := &peers[n]
		var enc, ap C.uint8_t
		first := C.int(0)
		if n == 0 {
			first = 1
		}
		err := check(C.seam_espnow_fetch_peer(first, (*C.uint8_t)(unsafe.Pointer(&p.MAC[0])), (*C.uint8_t)(unsafe.Pointer(&p.Channel)), &enc, &ap))
		if err == ErrESPNowNotFound {
			break
		}
		if err != nil {
			return n, err
		}
		p.Encrypted, p.AccessPoint = enc != 0, ap != 0
		n++
	}
	return n, nil
}

// PHYMode is wifi_phy_mode_t of ESP-IDF v5.3.2 esp_wifi_types_generic.h.
type PHYMode int32

const (
	PHYModeLR   PHYMode = 0 // Espressif Long Range
	PHYMode11B  PHYMode = 1
	PHYMode11G  PHYMode = 2
	PHYModeHT20 PHYMode = 4 // 802.11n, 20 MHz
)

// PHYRate is wifi_phy_rate_t of esp_wifi_types_generic.h for the ESP32-S3.
type PHYRate int32

const (
	PHYRate1ML     PHYRate = 0x00 // 1 Mbps, long preamble
	PHYRate2ML     PHYRate = 0x01
	PHYRate5ML     PHYRate = 0x02 // 5.5 Mbps
	PHYRate11ML    PHYRate = 0x03
	PHYRate2MS     PHYRate = 0x05 // 2 Mbps, short preamble
	PHYRate5MS     PHYRate = 0x06
	PHYRate11MS    PHYRate = 0x07
	PHYRate48M     PHYRate = 0x08
	PHYRate24M     PHYRate = 0x09
	PHYRate12M     PHYRate = 0x0a
	PHYRate6M      PHYRate = 0x0b
	PHYRate54M     PHYRate = 0x0c
	PHYRate36M     PHYRate = 0x0d
	PHYRate18M     PHYRate = 0x0e
	PHYRate9M      PHYRate = 0x0f
	PHYRateMCS0LGI PHYRate = 0x10 // MCS0 to MCS7 with long guard interval
	PHYRateMCS7LGI PHYRate = 0x17
	PHYRateMCS0SGI PHYRate = 0x18 // MCS0 to MCS7 with short guard interval
	PHYRateMCS7SGI PHYRate = 0x1f
	PHYRateLR250K  PHYRate = 0x29 // Espressif Long Range, 250 kbps
	PHYRateLR500K  PHYRate = 0x2a // Espressif Long Range, 500 kbps
)

// ESPNowSetPeerRate sets the PHY mode and rate of the messages to a peer, as
// esp_now_set_peer_rate_config. For PHYModeLR, call SetLongRange on the two
// devices.
func ESPNowSetPeerRate(mac [6]byte, mode PHYMode, rate PHYRate) error {
	defer releaseThreadSemaphore()
	return check(C.seam_espnow_rate((*C.uint8_t)(unsafe.Pointer(&mac[0])), C.int(mode), C.int(rate)))
}

// ESPNowSetWakeWindow sets how long the radio stays awake in each wake
// interval, both in milliseconds, for ESP-NOW with power save. As in ESP-IDF,
// it works only when the station is connected.
func ESPNowSetWakeWindow(window, interval uint16) error {
	defer releaseThreadSemaphore()
	if err := check(C.esp_wifi_connectionless_module_set_wake_interval(C.uint16_t(interval))); err != nil {
		return err
	}
	return check(C.esp_now_set_wake_window(C.uint16_t(window)))
}

// SetLongRange adds the Espressif Long Range protocol to the station, or
// removes it, as esp_wifi_set_protocol. Long Range frames go farther at 250
// or 500 kbps. Only Espressif chips receive them.
func SetLongRange(on bool) error {
	defer releaseThreadSemaphore()
	proto := C.uint8_t(0x1 | 0x2 | 0x4) // WIFI_PROTOCOL_11B, 11G and 11N
	if on {
		proto |= 0x8 // WIFI_PROTOCOL_LR
	}
	return check(C.esp_wifi_set_protocol(ifStation, proto))
}

// ESPNowSend sends data of up to ESPNowMaxData bytes to a peer and waits for
// the send status. A unicast message gives ErrESPNowNoAck when the peer did
// not acknowledge it. A broadcast message has no acknowledgement.
func ESPNowSend(mac [6]byte, data []byte) error {
	if len(data) == 0 || len(data) > ESPNowMaxData {
		return ErrInvalidArg
	}
	// esp_now.h: send the next message after the send callback of the last one.
	espnow.sendMu.Lock()
	defer espnow.sendMu.Unlock()
	select {
	case <-espnow.sent:
	default:
	}
	err := check(C.esp_now_send((*C.uint8_t)(unsafe.Pointer(&mac[0])), (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data))))
	releaseThreadSemaphore()
	if err != nil {
		return err
	}
	select {
	case ok := <-espnow.sent:
		if !ok {
			return ErrESPNowNoAck
		}
		return nil
	case <-time.After(time.Second):
		return ErrESPNowTimeout
	}
}

// HandleESPNow calls handler for each received message in a new goroutine.
// Call it once. It goes on after a restart.
func HandleESPNow(handler func(m *ESPNowMessage)) {
	espnowChannels()
	go func() {
		var m ESPNowMessage
		for msg := range espnow.msgs {
			b := msg.b
			if msg.gen != espnow.gen.Load() {
				continue // the buffer is of a pool that ESPNowDeinit freed
			}
			m = ESPNowMessage{Src: b.src, Dst: b.dst, RSSI: b.rssi, Data: b.data[:b.n]}
			handler(&m)
			if msg.gen != espnow.gen.Load() {
				continue
			}
			select {
			case espnow.free <- b:
			default:
			}
		}
	}()
}

// ESPNowDropped gives the number of messages that came when no buffer was free.
func ESPNowDropped() uint32 { return espnow.dropped.Load() }

// SetChannel sets the primary Wi-Fi channel, from 1 to 13, as
// esp_wifi_set_channel. It works only when the station is not connected.
func SetChannel(ch uint8) error {
	defer releaseThreadSemaphore()
	return check(C.esp_wifi_set_channel(C.uint8_t(ch), 0)) // WIFI_SECOND_CHAN_NONE
}

// Channel gives the primary Wi-Fi channel.
func Channel() (uint8, error) {
	defer releaseThreadSemaphore()
	var ch C.uint8_t
	var second C.int
	err := check(C.esp_wifi_get_channel(&ch, &second))
	return uint8(ch), err
}

// espnowRecv is the esp_now_recv_cb_t. It runs on the Wi-Fi task.
//
//export tinygo_espnow_recv
func espnowRecv(src, dst *byte, rssi int8, data *byte, n int32) {
	if n < 0 || n > ESPNowMaxData || espnow.msgs == nil {
		espnow.dropped.Add(1)
		return
	}
	select {
	case b := <-espnow.free:
		b.src, b.dst = [6]byte(unsafe.Slice(src, 6)), [6]byte(unsafe.Slice(dst, 6))
		b.rssi, b.n = rssi, int(n)
		copy(b.data[:], unsafe.Slice(data, n))
		espnow.msgs <- espnowMsg{b, espnow.gen.Load()}
	default:
		espnow.dropped.Add(1)
	}
}

// espnowSent is the esp_now_send_cb_t.
//
//export tinygo_espnow_sent
func espnowSent(mac *byte, ok bool) {
	select {
	case espnow.sent <- ok:
	default:
	}
}
