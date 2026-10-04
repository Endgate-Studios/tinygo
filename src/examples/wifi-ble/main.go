// Command wifi-ble runs Wi-Fi and Bluetooth LE at the same time on the
// ESP32-S3. It serves HTTP on Wi-Fi and scans for BLE advertisements. The page
// shows the scan count and the memory. The SSID and the password come from
// the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/wifi-ble
package main

import (
	"device/esp/radio"
	"runtime"
	"runtime/memory/arena"
	"sync"
	"sync/atomic"
	"time"

	"tinygo/net/bluetooth/btctl"
	"tinygo/net/ip"
	"tinygo/net/web"
)

var ssid, password string

var (
	socketMem [16 << 10]byte
	ringMem   [8 << 10]byte
	serverBuf [1024]byte
)

var ctl *btctl.Controller

// scanner counts the advertising reports.
type scanner struct {
	reports  atomic.Uint32
	complete chan uint16
}

func (s *scanner) HandleHCI(p []byte, now uint32) error {
	if len(p) < 4 || p[0] != 0x04 { // H4 event
		return nil
	}
	switch code, params := p[1], p[3:]; {
	case code == 0x0e && len(params) >= 4: // Command Complete
		if params[3] != 0 {
			println("hci: opcode", uint16(params[1])|uint16(params[2])<<8, "status", params[3])
		}
		select {
		case s.complete <- uint16(params[1]) | uint16(params[2])<<8:
		default:
		}
	case code == 0x3e && len(params) > 1 && params[0] == 0x02: // LE Advertising Report
		s.reports.Add(uint32(params[1]))
	}
	return nil
}

var scan = scanner{complete: make(chan uint16, 4)}

func main() {
	time.Sleep(2 * time.Second)
	if ssid == "" {
		println("set main.ssid and main.password with -ldflags -X")
		return
	}
	must("init", radio.Init())
	must("mode", radio.SetMode(radio.ModeStation))
	must("start", radio.Start())
	go keepConnected()
	must("connect", radio.Connect(ssid, password))
	<-connected
	mem, err := arena.NewHeap(socketMem[:])
	must("socket memory", err)
	stack := ip.New(radio.Station{}, mem)
	radio.HandleFrames(stack.Input)
	must("dhcp", stack.DHCP(30*time.Second))
	println("address", stack.Config().Addr.String())

	rings, err := arena.NewHeap(ringMem[:])
	must("ring memory", err)
	ctl, err = btctl.New(rings, radio.BLEDriver{}, &sync.Mutex{}, 4096)
	must("btctl", err)
	must("ble", radio.BLEEnable(func(p []byte) { ctl.Deliver(p) }))
	go func() {
		for {
			ctl.Poll(&scan, 0)
			time.Sleep(5 * time.Millisecond)
		}
	}()
	command([]byte{0x01, 0x03, 0x0c, 0x00}) // HCI Reset
	command([]byte{0x01, 0x01, 0x0c, 0x08,  // Set Event Mask: the default and LE Meta
		0xff, 0xff, 0xff, 0xff, 0xff, 0x1f, 0x00, 0x20})
	command([]byte{0x01, 0x0b, 0x20, 0x07, 0x00, // LE Set Scan Parameters: passive
		0xa0, 0x00, 0x30, 0x00, 0x00, 0x00}) // interval 100 ms, window 30 ms
	command([]byte{0x01, 0x0c, 0x20, 0x02, 0x01, 0x00}) // LE Set Scan Enable
	println("ble scanning, blob heap in use", radio.MemoryStats().InUse, "peak", radio.MemoryStats().Peak)

	l, err := stack.ListenTCP(80, 2048)
	must("listen", err)
	println("serving on port 80")
	must("serve", web.NewServer(l, serverBuf[:], handle).Serve())
}

// command sends one HCI command and waits for its Command Complete.
func command(p []byte) {
	for !(radio.BLEDriver{}).CanSend() {
		time.Sleep(time.Millisecond)
	}
	if err := ctl.WriteHCI(p); err != nil {
		println("hci:", err.Error())
		return
	}
	select {
	case <-scan.complete:
	case <-time.After(2 * time.Second):
		println("hci: no command complete")
	}
}

var ms runtime.MemStats // global: a local one escapes to the heap

func handle(w *web.Response, r *web.Request) {
	runtime.ReadMemStats(&ms)
	blob := radio.MemoryStats()
	w.WriteString("ble reports ")
	w.WriteUint(uint64(scan.reports.Load()))
	w.WriteString(", mallocs ")
	w.WriteUint(ms.Mallocs)
	w.WriteString(", gc heap in use ")
	w.WriteUint(ms.HeapInuse)
	w.WriteString(", blob heap in use ")
	w.WriteUint(uint64(blob.InUse))
	w.WriteString(" peak ")
	w.WriteUint(uint64(blob.Peak))
	w.WriteString(" size ")
	w.WriteUint(uint64(blob.Size))
	w.WriteString("\n")
}

// connected gets a value at each connection of the station.
var connected = make(chan struct{}, 1)

// keepConnected reads the radio events. After a disconnect, for example when
// the access point restarts, it waits one second and connects again.
func keepConnected() {
	for e := range radio.Events() {
		switch e.ID {
		case radio.EventStaConnected:
			select {
			case connected <- struct{}{}:
			default:
			}
		case radio.EventStaDisconnected:
			println("wifi: disconnected, reason", e.DisconnectReason())
			time.Sleep(time.Second)
			if err := radio.Reconnect(); err != nil {
				println("wifi: reconnect:", err.Error())
			}
		}
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
