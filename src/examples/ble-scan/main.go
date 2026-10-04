// Command ble-scan starts the Bluetooth LE controller of the ESP32-S3 and
// scans for advertisements with raw HCI commands.
//
//	tinygo flash -target esp32s3-psram-octal ./src/examples/ble-scan
package main

import (
	"device/esp/radio"
	"runtime/memory/arena"
	"sync"
	"time"

	"tinygo/net/bluetooth/btctl"
)

var ringMem [8 << 10]byte

var ctl *btctl.Controller

// host prints what the controller sends.
type host struct {
	reports  int
	shown    int
	complete chan uint16 // opcodes of Command Complete events
}

func (h *host) HandleHCI(p []byte, now uint32) error {
	if len(p) < 3 || p[0] != 0x04 { // H4 event
		return nil
	}
	switch code, params := p[1], p[3:]; {
	case code == 0x0e && len(params) >= 4: // Command Complete
		op := uint16(params[1]) | uint16(params[2])<<8
		println("command complete", hex16(op), "status", params[3])
		if op == 0x1009 && len(params) >= 10 { // Read BD_ADDR
			a := params[4:10]
			println("BD_ADDR", a[5], a[4], a[3], a[2], a[1], a[0])
		}
		select {
		case h.complete <- op:
		default:
		}
	case code == 0x0f && len(params) >= 4: // Command Status
		println("command status", hex16(uint16(params[2])|uint16(params[3])<<8), "status", params[0])
	case code == 0x3e && len(params) > 0 && params[0] == 0x02: // LE Advertising Report
		n := int(params[1])
		h.reports += n
		if h.shown < 8 && len(params) >= 12 {
			a := params[4:10]
			rssi := int8(params[len(params)-1])
			println("  adv from", a[5], a[4], a[3], a[2], a[1], a[0], "rssi", rssi)
			h.shown++
		}
	}
	return nil
}

func hex16(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{'0', 'x', digits[v>>12], digits[v>>8&15], digits[v>>4&15], digits[v&15]})
}

func main() {
	time.Sleep(2 * time.Second)
	mem, err := arena.NewHeap(ringMem[:])
	must("ring memory", err)
	ctl, err = btctl.New(mem, radio.BLEDriver{}, &sync.Mutex{}, 4096)
	must("btctl", err)
	must("ble", radio.BLEEnable(func(p []byte) { ctl.Deliver(p) }))
	println("controller enabled, heap in use", radio.MemoryStats().InUse)

	h := &host{complete: make(chan uint16, 4)}
	go func() {
		for {
			ctl.Poll(h, 0)
			time.Sleep(5 * time.Millisecond)
		}
	}()

	command(h, []byte{0x01, 0x03, 0x0c, 0x00}) // HCI Reset
	command(h, []byte{0x01, 0x09, 0x10, 0x00}) // Read BD_ADDR
	command(h, []byte{0x01, 0x01, 0x0c, 0x08,  // Set Event Mask: the default and LE Meta
		0xff, 0xff, 0xff, 0xff, 0xff, 0x1f, 0x00, 0x20}) // LE Meta is bit 61
	command(h, []byte{0x01, 0x0b, 0x20, 0x07, 0x01, // LE Set Scan Parameters: active
		0xa0, 0x00, 0x50, 0x00, 0x00, 0x00}) // interval 100 ms, window 50 ms, public, all
	command(h, []byte{0x01, 0x0c, 0x20, 0x02, 0x01, 0x00}) // LE Set Scan Enable
	time.Sleep(10 * time.Second)
	println("advertising reports in 10 s:", h.reports, "dropped", ctl.Dropped(), "| heap in use", radio.MemoryStats().InUse, "peak", radio.MemoryStats().Peak)
}

// command sends p and waits for its Command Complete.
func command(h *host, p []byte) {
	for !(radio.BLEDriver{}).CanSend() {
		time.Sleep(time.Millisecond)
	}
	if err := ctl.WriteHCI(p); err != nil {
		println("send:", err.Error())
		return
	}
	select {
	case <-h.complete:
	case <-time.After(2 * time.Second):
		println("no command complete for", hex16(uint16(p[1])|uint16(p[2])<<8))
	}
}

func must(what string, err error) {
	if err != nil {
		println(what+":", err.Error())
		for {
			time.Sleep(time.Hour)
		}
	}
}
