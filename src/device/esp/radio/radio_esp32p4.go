//go:build esp32p4

package radio

import "errors"

// The ESP32-P4 has no radio. On a board with an ESP32-C6, the radio is on the C6, through an SDIO
// host and the esp-hosted protocol. Until that driver exists, this is the same API as on the chips
// with a radio, and it says there is none.

// ErrNoRadio is what every call gives.
var ErrNoRadio = errors.New("radio: the driver for the ESP32-C6 over SDIO is not written yet")

// FrameSize is the size of a received frame's buffer.
const FrameSize = 1600

// Mode is the Wi-Fi mode.
type Mode int32

const (
	ModeNull Mode = iota
	ModeStation
	ModeAccessPoint
	ModeStationAndAccessPoint
)

// PowerSave is the modem power save of the station.
type PowerSave int32

const (
	PowerSaveNone PowerSave = iota
	PowerSaveMinModem
	PowerSaveMaxModem
)

// AuthMode is how a network is secured.
type AuthMode int32

const AuthOpen AuthMode = 0

// AP is a network that a scan heard.
type AP struct {
	SSID    string
	BSSID   [6]byte
	Channel uint8
	RSSI    int8
	Auth    AuthMode
}

// EventID is a Wi-Fi event.
type EventID int32

const (
	EventStaConnected    EventID = 4
	EventStaDisconnected EventID = 5
)

// Event is a Wi-Fi event.
type Event struct {
	ID   EventID
	Data []byte
}

// DisconnectReason gives why the station disconnected.
func (e Event) DisconnectReason() uint8 { return 0 }

// AccessPoint gives the BSSID and channel of an EventStaConnected.
func (e Event) AccessPoint() (bssid [6]byte, channel uint8) { return }

// Frame is a received Ethernet frame.
type Frame struct{}

// Data gives the frame from the Ethernet header.
func (f Frame) Data() []byte { return nil }

// Release gives the buffer back.
func (f Frame) Release() {}

// Option is an option of Init.
type Option func()

// WithArenaMemory gives Init memory for the radio.
func WithArenaMemory(mem []byte) Option { return func() {} }

// WithPSRAMArena gives Init PSRAM for the radio.
func WithPSRAMArena(mem []byte) Option { return func() {} }

// Station is the station interface, for a network stack.
type Station struct{}

// MAC gives the MAC address of the station.
func (Station) MAC() [6]byte { return MAC() }

// Send sends an Ethernet frame.
func (Station) Send(frame []byte) error { return ErrNoRadio }

var (
	events = make(chan Event)
	frames = make(chan Frame)
)

func Init(opts ...Option) error                                         { return ErrNoRadio }
func SetMode(m Mode) error                                              { return ErrNoRadio }
func Start() error                                                      { return ErrNoRadio }
func Scan() ([]AP, error)                                               { return nil, ErrNoRadio }
func Connect(ssid, password string) error                               { return ErrNoRadio }
func ConnectTo(ssid, password string, bssid [6]byte, channel uint8) error { return ErrNoRadio }
func Signal() (int8, error)                                             { return 0, ErrNoRadio }
func Disconnect() error                                                 { return ErrNoRadio }
func SetPowerSave(ps PowerSave) error                                   { return ErrNoRadio }
func Events() <-chan Event                                              { return events }
func Frames() <-chan Frame                                              { return frames }
func FramesDropped() uint32                                             { return 0 }
func MAC() [6]byte                                                      { return [6]byte{} }
func Send(frame []byte) error                                           { return ErrNoRadio }
