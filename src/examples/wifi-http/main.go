// Command wifi-http gets a page and serves HTTP on the ESP32-S3 Wi-Fi with
// tinygo/net/web, which does not allocate for each request. The SSID and the
// password come from the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/wifi-http
package main

import (
	"device/esp/radio"
	"runtime"
	"runtime/memory/arena"
	"time"
	"tinygo/net/ip"
	"tinygo/net/web"
)

var ssid, password string

// psram is "1" to put the Wi-Fi buffers that need no DMA in PSRAM. Set it with
// -X main.psram=1.
var psram string

// arenaKB sets the internal Wi-Fi heap in KB, for example -X main.arenaKB=28.
// Empty keeps radio.DefaultArenaSize.
var arenaKB string

// socketMem holds the socket rings of the stack.
var socketMem [16 << 10]byte

// Buffers for the request and response heads.
var (
	clientBuf [1024]byte
	serverBuf [1024]byte
)

// counter counts the body bytes of the client. A pointer to a global value in
// an io.Writer does not allocate.
type counter struct{ n int }

func (c *counter) Write(p []byte) (int, error) {
	c.n += len(p)
	return len(p), nil
}

var body counter

func main() {
	time.Sleep(2 * time.Second)
	if ssid == "" {
		println("set main.ssid and main.password with -ldflags -X")
		return
	}
	var opts []radio.Option
	if psram == "1" && psramBuf() != nil {
		opts = append(opts, radio.WithPSRAMArena(psramBuf()))
		println("Wi-Fi buffers in PSRAM")
	}
	if kb := atoi(arenaKB); kb > 0 {
		opts = append(opts, radio.WithArenaSize(kb*1024))
		println("internal Wi-Fi heap", kb, "KB")
	}
	must("init", radio.Init(opts...))
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

	status, err := web.Get(stack, "example.com", 80, "/", clientBuf[:], &body)
	if err != nil {
		println("get:", err.Error())
	} else {
		println("get:", status, body.n, "bytes")
	}

	l, err := stack.ListenTCP(80, 2048)
	must("listen", err)
	println("serving on port 80")
	must("serve", web.NewServer(l, serverBuf[:], handle).Serve())
}

var (
	requests int
	ms       runtime.MemStats // global: a local one escapes to the heap
)

// handle answers with the request count and the GC counters. It allocates
// nothing.
func handle(w *web.Response, r *web.Request) {
	requests++
	if r.PathIs("/gc") {
		runtime.GC()
	}
	runtime.ReadMemStats(&ms)
	w.WriteString("hello from TinyGo on the ESP32-S3\nrequest ")
	w.WriteInt(int64(requests))
	w.WriteString(", path ")
	w.Write(r.Path)
	w.WriteString(", mallocs ")
	w.WriteUint(ms.Mallocs)
	w.WriteString(", heap in use ")
	w.WriteUint(ms.HeapInuse)
	internal, ext := radio.MemoryStats(), radio.PSRAMStats()
	w.WriteString(", wifi internal ")
	w.WriteUint(uint64(internal.InUse))
	w.WriteString(" peak ")
	w.WriteUint(uint64(internal.Peak))
	w.WriteString(", wifi psram ")
	w.WriteUint(uint64(ext.InUse))
	w.WriteString(" peak ")
	w.WriteUint(uint64(ext.Peak))
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

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
