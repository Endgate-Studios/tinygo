// Command wifi-ap makes an ESP32-S3 a Wi-Fi access point with WPA2-PSK. It
// gives the stations an address with DHCP and serves HTTP on 192.168.4.1. Set
// the SSID and the password of the access point at the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.apSSID=..." -X "main.apPassword=..."' ./src/examples/wifi-ap
package main

import (
	"device/esp/radio"
	"runtime/memory/arena"
	"sync/atomic"
	"time"

	"tinygo/net/ip"
	"tinygo/net/web"
)

// apSSID and apPassword are the network of the access point. An empty
// password makes an open access point.
var (
	apSSID     = "tinygo-ap"
	apPassword = "tinygo-ap-test"
)

var (
	socketMem [32 << 10]byte
	serverBuf [1024]byte
)

var stations, joins atomic.Int32

func main() {
	time.Sleep(2 * time.Second)
	must("init", radio.Init())
	must("mode", radio.SetMode(radio.ModeAccessPoint))
	must("configure", radio.ConfigureAP(radio.APConfig{SSID: apSSID, Password: apPassword, Channel: 6}))
	must("start", radio.Start())
	go events()

	mem, err := arena.NewHeap(socketMem[:])
	must("socket memory", err)
	stack := ip.New(radio.AccessPoint{}, mem)
	stack.SetConfig(ip.Config{Addr: ip.Addr{192, 168, 4, 1}, Mask: ip.Addr{255, 255, 255, 0}})
	radio.HandleAPFrames(stack.Input)
	go func() {
		must("dhcp server", stack.ServeDHCP(ip.Addr{192, 168, 4, 2}, radio.MaxStations, time.Hour))
	}()
	println("access point", apSSID, "on 192.168.4.1")

	l, err := stack.ListenTCP(80, 2048)
	must("listen", err)
	println("serving on port 80")
	must("serve", web.NewServer(l, serverBuf[:], handle).Serve())
}

// events prints the stations that join and leave.
func events() {
	for e := range radio.Events() {
		switch e.ID {
		case radio.EventAPStaConnected:
			stations.Add(1)
			joins.Add(1)
			println("station joined", hexMAC(e.Data))
		case radio.EventAPStaDisconnected:
			stations.Add(-1)
			println("station left", hexMAC(e.Data))
		}
	}
}

func handle(w *web.Response, r *web.Request) {
	w.WriteString("hello from the TinyGo access point, stations ")
	w.WriteUint(uint64(stations.Load()))
	w.WriteString(", joins ")
	w.WriteUint(uint64(joins.Load()))
	w.WriteString("\n")
}

// hexMAC formats the MAC address at the start of an event.
func hexMAC(b []byte) string {
	if len(b) < 6 {
		return "?"
	}
	const digits = "0123456789abcdef"
	var s [17]byte
	for i := range 6 {
		s[3*i], s[3*i+1] = digits[b[i]>>4], digits[b[i]&15]
		if i < 5 {
			s[3*i+2] = ':'
		}
	}
	return string(s[:])
}

func must(what string, err error) {
	if err != nil {
		println(what+":", err.Error())
		for {
			time.Sleep(time.Hour)
		}
	}
}
