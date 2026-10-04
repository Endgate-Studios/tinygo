//go:build esp32s3

// Package radio runs the ESP32-S3 Wi-Fi and BLE blobs of ESP-IDF v5.3.2. The
// blobs are the git submodules in blobs/.
//
// The cgo seam is small. The C files (seam.c and the generated osi_table.c)
// have only what Go cannot do: the table of OS functions, variadic
// functions and the symbols that the blobs use by name. Go implements the
// OS functions, and a runtime/memory arena.Heap owns all memory of the blobs
// (memory.go).
package radio

// TinyGo reads the #cgo lines itself. It does not expand ${SRCDIR}, but it
// makes relative -I and -L paths relative to the package directory.

/*
#cgo CFLAGS: -Iinclude -fno-short-enums
#cgo LDFLAGS: -Lblobs/esp32-wifi-lib/esp32s3
#cgo LDFLAGS: -Lblobs/esp-phy-lib/esp32s3
#cgo LDFLAGS: -Lblobs/esp-coex-lib/esp32s3
#cgo LDFLAGS: -lnet80211 -lpp -lcore -lphy -lcoexist
#include "seam.h"
*/
import "C"

import (
	"runtime/memory"
	"strconv"
	"unsafe"
)

// Error is an esp_err_t code of the blobs.
type Error int32

// Errors of ESP-IDF v5.3.2 esp_err.h.
const (
	ErrNoMem      Error = 0x101  // ESP_ERR_NO_MEM
	ErrInvalidArg Error = 0x102  // ESP_ERR_INVALID_ARG
	ErrNotInit    Error = 0x3001 // ESP_ERR_WIFI_NOT_INIT
)

// ifStation is WIFI_IF_STA of wifi_interface_t.
const ifStation = 0

func (e Error) Error() string {
	return "wifi: error 0x" + strconv.FormatInt(int64(e), 16)
}

func check(r C.int) error {
	if r != 0 {
		return Error(r)
	}
	return nil
}

// Init starts the Wi-Fi driver of the blobs, as esp_wifi_init.
// Init makes the blob heap and initializes the blobs. The options set the
// heap, see WithArenaSize and WithArenaMemory.
func Init(opts ...Option) error {
	defer releaseThreadSemaphore()
	if err := initMemory(applyOptions(DefaultArenaSize, opts)); err != nil {
		return err
	}
	startService()
	powerDomainOn()
	if err := check(C.seam_coex_init()); err != nil {
		return err
	}
	return check(C.seam_wifi_init(psramHeap != nil))
}

// Mode is the Wi-Fi mode, as wifi_mode_t.
type Mode int32

const (
	ModeNull Mode = iota
	ModeStation
	ModeAccessPoint
	ModeStationAndAccessPoint
)

// SetMode sets the Wi-Fi mode, as esp_wifi_set_mode.
func SetMode(m Mode) error {
	defer releaseThreadSemaphore()
	return check(C.esp_wifi_set_mode(C.int(m)))
}

// Start starts Wi-Fi in the current mode, as esp_wifi_start.
func Start() error {
	defer releaseThreadSemaphore()
	if err := startRX(); err != nil {
		return err
	}
	return check(C.esp_wifi_start())
}

// AP is an access point that Scan found.
type AP struct {
	SSID    string
	BSSID   [6]byte
	Channel uint8
	RSSI    int8
	Auth    int // wifi_auth_mode_t
}

// Scan does an active scan of all channels. It waits until the scan is done.
func Scan() ([]AP, error) {
	defer releaseThreadSemaphore()
	if err := check(C.esp_wifi_scan_start(nil, true)); err != nil {
		return nil, err
	}
	var n C.uint16_t
	if err := check(C.esp_wifi_scan_get_ap_num(&n)); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	records := memory.MakeSlice[C.wifi_ap_record_t](blobHeap, int(n))
	if records == nil {
		return nil, ErrNoMem
	}
	defer memory.FreeSlice(blobHeap, records)
	if err := check(C.esp_wifi_scan_get_ap_records(&n, &records[0])); err != nil {
		return nil, err
	}
	aps := make([]AP, n)
	for i, r := range records[:n] {
		ssid := unsafe.Slice((*byte)(unsafe.Pointer(&r.ssid[0])), len(r.ssid))
		for j, b := range ssid {
			if b == 0 {
				ssid = ssid[:j]
				break
			}
		}
		aps[i] = AP{
			SSID:    string(ssid),
			Channel: uint8(r.primary),
			RSSI:    int8(r.rssi),
			Auth:    int(r.authmode),
		}
		copy(aps[i].BSSID[:], unsafe.Slice((*byte)(unsafe.Pointer(&r.bssid[0])), 6))
	}
	return aps, nil
}

// AuthWPA2PSK is WIFI_AUTH_WPA2_PSK of wifi_auth_mode_t.
const AuthWPA2PSK = 3

// Connect starts to connect the station to a network. An empty password
// selects an open network, and other networks need WPA2-PSK with CCMP. The
// password is a passphrase of 8 to 63 characters or a PSK of 64 hex digits. EventStaConnected tells when it is
// connected.
func Connect(ssid, password string) error {
	defer releaseThreadSemaphore()
	var conf C.wifi_config_t
	sta := (*C.wifi_sta_config_t)(unsafe.Pointer(&conf))
	// A 64-digit PSK fills the password field without a NUL, as in ESP-IDF.
	if len(ssid) > len(sta.ssid) || len(password) > len(sta.password) {
		return ErrInvalidArg
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&sta.ssid[0])), len(sta.ssid)), ssid)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&sta.password[0])), len(sta.password)), password)
	if password != "" {
		sta.threshold.authmode = AuthWPA2PSK
	}
	if err := prepareSupplicant(ssid, password); err != nil {
		return err
	}
	if err := check(C.esp_wifi_set_config(ifStation, &conf)); err != nil {
		return err
	}
	return check(C.esp_wifi_connect_internal())
}

// Reconnect connects the station again with the configuration of the last
// Connect, as esp_wifi_connect. Call it after EventStaDisconnected.
func Reconnect() error {
	defer releaseThreadSemaphore()
	return check(C.esp_wifi_connect_internal())
}

// Disconnect disconnects the station.
func Disconnect() error {
	defer releaseThreadSemaphore()
	return check(C.esp_wifi_disconnect_internal())
}

// PowerSave is a wifi_ps_type_t of ESP-IDF v5.3.2 esp_wifi_types_generic.h.
type PowerSave int32

const (
	PowerSaveNone     PowerSave = iota // the radio is always on
	PowerSaveMinModem                  // wake for each DTIM beacon, the ESP-IDF default
	PowerSaveMaxModem                  // wake for each listen interval
)

// SetPowerSave sets the modem power save of the station. The default is
// PowerSaveMinModem. PowerSaveNone gives less delay and uses more current.
func SetPowerSave(ps PowerSave) error {
	defer releaseThreadSemaphore()
	return check(C.esp_wifi_set_ps(C.int(ps)))
}
