//go:build esp32s3

package radio

import (
	"sync/atomic"
	"unsafe"
)

// EventID is a wifi_event_t of ESP-IDF v5.3.2 esp_wifi_types_generic.h.
type EventID int32

// Some events of wifi_event_t.
const (
	EventScanDone          EventID = 1
	EventStaStart          EventID = 2
	EventStaStop           EventID = 3
	EventStaConnected      EventID = 4
	EventStaDisconnected   EventID = 5
	EventStaAuthModeChange EventID = 6
	EventAPStart           EventID = 12
	EventAPStop            EventID = 13
	EventAPStaConnected    EventID = 14
	EventAPStaDisconnected EventID = 15
	EventHomeChannelChange EventID = 43
)

// Event is an event of the blobs. Data is a copy of the event struct, for
// example wifi_event_sta_connected_t.
type Event struct {
	ID   EventID
	Data []byte
}

// DisconnectReason gives the wifi_err_reason_t of an EventStaDisconnected, for
// example 8 when Disconnect was called and 200 when the beacons stopped.
func (e Event) DisconnectReason() uint8 {
	// The reason field of wifi_event_sta_disconnected_t.
	if e.ID != EventStaDisconnected || len(e.Data) < 40 {
		return 0
	}
	return e.Data[39]
}

var (
	events        = make(chan Event, 16)
	eventsDropped atomic.Uint32
)

// staConnected tells whether the station is connected: from EventStaConnected
// to EventStaDisconnected, EventStaStop, Stop or Deinit. osiEventPost sets it
// for all events, also the ones that Events drops. TSF uses it.
var staConnected atomic.Bool

// Events gives the events of the blobs. Read it often: when it is full, new
// events are dropped, see EventsDropped.
func Events() <-chan Event { return events }

// EventsDropped gives the number of events that Events did not have space for.
func EventsDropped() uint32 { return eventsDropped.Load() }

// osiEventPost copies the data, because the blobs free it after the call.
// It does not wait, so a slow reader cannot stop the Wi-Fi task.
//
//export tinygo_wifi_event_post
func osiEventPost(base *byte, id int32, data unsafe.Pointer, size uintptr, ticks uint32) int32 {
	e := Event{ID: EventID(id)}
	switch e.ID {
	case EventStaConnected:
		staConnected.Store(true)
	case EventStaDisconnected, EventStaStop:
		staConnected.Store(false)
	}
	if size > 0 && data != nil {
		e.Data = append([]byte(nil), unsafe.Slice((*byte)(data), size)...)
	}
	select {
	case events <- e:
	default:
		eventsDropped.Add(1)
	}
	return 0
}
