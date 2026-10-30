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

// framePool is the number of received frames that wait for the reader:
// enough to ride out the reader being busy for a while (a card write, say)
// at full speed, rather than drop frames for TCP to send again.
const framePool = 32

// ifAP is WIFI_IF_AP of esp_wifi_types_generic.h.
const ifAP = 1

// Frame is a received Ethernet frame. Release it when you no longer use Data.
type Frame struct {
	buf  *[FrameSize]byte
	n    int
	gen  uint32
	path *rxPath
}

// Data gives the frame from the Ethernet header.
func (f Frame) Data() []byte { return f.buf[:f.n] }

// Release gives the buffer back to the pool.
func (f Frame) Release() {
	if f.path == nil || f.gen != f.path.gen.Load() {
		return // the buffer is of a frame pool that Deinit freed
	}
	select {
	case f.path.free <- f.buf:
	default:
	}
}

// rxPath is the frame pool and the frames of one interface.
type rxPath struct {
	frames  chan Frame
	free    chan *[FrameSize]byte
	pool    unsafe.Pointer // the buffers in the blob heap, or nil
	gen     atomic.Uint32  // changes when Deinit frees the pool
	dropped atomic.Uint32
}

var rx, rxAP rxPath

// start makes the frame pool in the blob heap.
func (p *rxPath) start() error {
	if p.pool != nil {
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
	if p.frames == nil {
		p.free = make(chan *[FrameSize]byte, framePool)
		p.frames = make(chan Frame, framePool)
	}
	p.pool = unsafe.Pointer(&bufs[0])
	for i := range bufs {
		p.free <- &bufs[i]
	}
	return nil
}

// stop frees the frame pool after the deinit of the blobs. The channels stay,
// so HandleFrames goes on after the next Start.
func (p *rxPath) stop() {
	if p.pool == nil {
		return
	}
	p.gen.Add(1)
	for len(p.free) > 0 {
		<-p.free
	}
	for len(p.frames) > 0 {
		<-p.frames
	}
	heapOf(p.pool).Free(p.pool)
	p.pool = nil
}

// receive copies a frame of the blobs and frees the buffer of the blobs at
// once, because the blobs have few of them.
func (p *rxPath) receive(buffer unsafe.Pointer, n uint16, eb unsafe.Pointer) {
	defer C.esp_wifi_internal_free_rx_buffer(eb)
	if int(n) > FrameSize || p.frames == nil {
		p.dropped.Add(1)
		return
	}
	select {
	case buf := <-p.free:
		copy(buf[:], unsafe.Slice((*byte)(buffer), n))
		p.frames <- Frame{buf, int(n), p.gen.Load(), p}
	default:
		p.dropped.Add(1)
	}
}

func (p *rxPath) handle(handler func(frame []byte)) {
	go func() {
		for f := range p.frames {
			handler(f.Data())
			f.Release()
		}
	}()
}

// startRX makes the frame pools and registers the RX callbacks of the
// interfaces of the mode.
func startRX() error {
	if mode == ModeStation || mode == ModeStationAndAccessPoint {
		if err := rx.start(); err != nil {
			return err
		}
		if err := check(C.seam_wifi_reg_rxcb()); err != nil {
			return err
		}
	}
	if mode == ModeAccessPoint || mode == ModeStationAndAccessPoint {
		if err := rxAP.start(); err != nil {
			return err
		}
		return check(C.seam_wifi_reg_rxcb_ap())
	}
	return nil
}

func stopRX() {
	rx.stop()
	rxAP.stop()
}

// Frames gives the Ethernet frames that the station receives.
func Frames() <-chan Frame { return rx.frames }

// HandleFrames calls handler for each Ethernet frame of the station in a new
// goroutine, for example the Input method of a tinygo/net/ip Stack. The frame
// is valid only during the call. Call it after Start.
func HandleFrames(handler func(frame []byte)) { rx.handle(handler) }

// HandleAPFrames is HandleFrames for the frames of the access point.
func HandleAPFrames(handler func(frame []byte)) { rxAP.handle(handler) }

// FramesDropped gives the number of frames that came when no buffer was free.
func FramesDropped() uint32 { return rx.dropped.Load() + rxAP.dropped.Load() }

// wifiRX is the wifi_rxcb_t of the station.
//
//export tinygo_wifi_rx
func wifiRX(buffer unsafe.Pointer, n uint16, eb unsafe.Pointer) int32 {
	rx.receive(buffer, n, eb)
	return 0
}

// wifiRXAP is the wifi_rxcb_t of the access point.
//
//export tinygo_wifi_rx_ap
func wifiRXAP(buffer unsafe.Pointer, n uint16, eb unsafe.Pointer) int32 {
	rxAP.receive(buffer, n, eb)
	return 0
}

// Send sends an Ethernet frame from the station. The blobs copy it.
func Send(frame []byte) error { return send(ifStation, frame) }

func send(ifx C.int, frame []byte) error {
	if len(frame) == 0 || len(frame) > FrameSize {
		return ErrInvalidArg
	}
	return check(C.esp_wifi_internal_tx(ifx, unsafe.Pointer(&frame[0]), C.uint16_t(len(frame))))
}

// Station is the station as an Ethernet link. It has the MAC and Send methods
// of the Link of tinygo/net/ip.
type Station struct{}

// MAC gives the station MAC address, which is the base MAC of the eFuse.
func (Station) MAC() [6]byte { return MAC() }

// Send sends an Ethernet frame from the station.
func (Station) Send(frame []byte) error { return Send(frame) }

// AccessPoint is the soft-AP as an Ethernet link, as Station.
type AccessPoint struct{}

// MAC gives the MAC address of the access point. Call it after Init.
func (AccessPoint) MAC() [6]byte {
	var mac [6]byte
	C.esp_wifi_get_macaddr_internal(ifAP, (*C.uint8_t)(unsafe.Pointer(&mac[0])))
	return mac
}

// Send sends an Ethernet frame from the access point.
func (AccessPoint) Send(frame []byte) error { return send(ifAP, frame) }
