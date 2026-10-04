//go:build esp32s3

package radio

/*
#include "seam.h"
*/
import "C"

import (
	"runtime/memory"
	"sync/atomic"
	"unsafe"
)

// FrameSize is the size of a frame buffer. It holds an Ethernet frame of
// 1514 bytes.
const FrameSize = 1536

// framePool is the number of received frames that wait for the reader.
const framePool = 8

// Frame is a received Ethernet frame. Release it when you no longer use Data.
type Frame struct {
	buf *[FrameSize]byte
	n   int
}

// Data gives the frame from the Ethernet header.
func (f Frame) Data() []byte { return f.buf[:f.n] }

// Release gives the buffer back to the pool.
func (f Frame) Release() {
	select {
	case rx.free <- f.buf:
	default:
	}
}

var rx struct {
	frames  chan Frame
	free    chan *[FrameSize]byte
	dropped atomic.Uint32
}

// startRX makes the frame pool in the Wi-Fi arena and registers the RX
// callback of the station.
func startRX() error {
	if rx.frames != nil {
		return nil
	}
	// Only the CPU copies frames, so the pool can be in PSRAM.
	h := blobHeap
	if psramHeap != nil {
		h = psramHeap
	}
	bufs := memory.MakeSlice[[FrameSize]byte](h, framePool)
	if bufs == nil {
		return ErrNoMem
	}
	rx.free = make(chan *[FrameSize]byte, framePool)
	rx.frames = make(chan Frame, framePool)
	for i := range bufs {
		rx.free <- &bufs[i]
	}
	return check(C.seam_wifi_reg_rxcb())
}

// Frames gives the Ethernet frames that the station receives.
func Frames() <-chan Frame { return rx.frames }

// HandleFrames calls handler for each received Ethernet frame in a new
// goroutine, for example the Input method of a tinygo/net/ip Stack. The frame
// is valid only during the call.
func HandleFrames(handler func(frame []byte)) {
	go func() {
		for f := range rx.frames {
			handler(f.Data())
			f.Release()
		}
	}()
}

// FramesDropped gives the number of frames that came when no buffer was free.
func FramesDropped() uint32 { return rx.dropped.Load() }

// wifiRX is the wifi_rxcb_t of the station. It copies the frame and frees
// the buffer of the blobs at once, because the blobs have few of them.
//
//export tinygo_wifi_rx
func wifiRX(buffer unsafe.Pointer, n uint16, eb unsafe.Pointer) int32 {
	defer C.esp_wifi_internal_free_rx_buffer(eb)
	if int(n) > FrameSize {
		rx.dropped.Add(1)
		return 0
	}
	select {
	case buf := <-rx.free:
		copy(buf[:], unsafe.Slice((*byte)(buffer), n))
		rx.frames <- Frame{buf, int(n)}
	default:
		rx.dropped.Add(1)
	}
	return 0
}

// Send sends an Ethernet frame from the station. The blobs copy it.
func Send(frame []byte) error {
	if len(frame) == 0 || len(frame) > FrameSize {
		return ErrInvalidArg
	}
	return check(C.esp_wifi_internal_tx(ifStation, unsafe.Pointer(&frame[0]), C.uint16_t(len(frame))))
}

// Station is the station as an Ethernet link. It has the MAC and Send methods
// of the Link of tinygo/net/ip.
type Station struct{}

// MAC gives the station MAC address, which is the base MAC of the eFuse.
func (Station) MAC() [6]byte { return MAC() }

// Send sends an Ethernet frame from the station.
func (Station) Send(frame []byte) error { return Send(frame) }
