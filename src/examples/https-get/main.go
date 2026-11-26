// Command https-get gets https://www.google.com/ two times with the client of
// tinygo/net/https. It trusts the P-384 key of GTS Root R4, and the second
// request resumes the TLS session. The SSID and the password come from the
// link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/https-get
package main

import (
	"device/esp"
	"device/esp/radio"
	"encoding/hex"
	"runtime/memory/arena"
	"time"

	"tinygo/net/https"
	"tinygo/net/ip"
	"tinygo/net/tls"
)

var ssid, password string

var (
	socketMem [32 << 10]byte
	tlsMem    [40 << 10]byte
)

// The key of GTS Root R4, x and y.
const r4 = "f37473a7688b60ae43b835c581307b4b499dfbc161cee6de46bd6bd5611835ae40dd73f78991305aeb3cee857ca24076" +
	"3ba9c6b847d82ae792916a73e9b172399f299fa298d35f5e5886650fa1846506d1dc8bc9c773c88c6a2fe5c4abd11d8a"

// rng reads the hardware random number generator, a true random source
// while Wi-Fi runs (ESP32-S3 Technical Reference Manual, chapter Random
// Number Generator).
type rng struct{}

func (rng) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = byte(esp.RNG.DATA.Get())
	}
	return len(b), nil
}

// counter counts the bytes of the body.
type counter int

func (c *counter) Write(b []byte) (int, error) {
	*c += counter(len(b))
	return len(b), nil
}

func main() {
	time.Sleep(2 * time.Second)
	if ssid == "" {
		println("set main.ssid and main.password with -ldflags -X")
		return
	}
	must("init", radio.Init())
	must("mode", radio.SetMode(radio.ModeStation))
	must("start", radio.Start())
	go func() {
		for e := range radio.Events() {
			if e.ID == radio.EventStaConnected {
				select {
				case connected <- struct{}{}:
				default:
				}
			}
		}
	}()
	must("connect", radio.Connect(ssid, password))
	<-connected
	mem, err := arena.NewHeap(socketMem[:])
	must("socket memory", err)
	stack := ip.New(radio.Station{}, mem)
	radio.HandleFrames(stack.Input)
	must("dhcp", stack.DHCP(30*time.Second))

	ntp, err := stack.LookupHost("pool.ntp.org", 5*time.Second)
	must("lookup ntp", err)
	now, err := stack.SNTP(ntp, 5*time.Second)
	must("sntp", err)
	boot := time.Now()

	tm, err := arena.NewHeap(tlsMem[:])
	must("tls memory", err)
	root := tls.PublicKey{P384: make([]byte, 96)}
	hex.Decode(root.P384, []byte(r4))
	client := &https.Client{
		Stack: stack,
		Mem:   tm,
		Rand:  rng{},
		TLS:   tls.Config{Roots: []tls.PublicKey{root}, Session: &tls.Session{}},
		Now:   func() time.Time { return now.Add(time.Since(boot)) },
	}
	for i := 0; i < 2; i++ {
		var n counter
		t0 := time.Now()
		status, err := client.Get("www.google.com", "/", &n)
		must("get", err)
		println("status", status, "bytes", int(n), "ms", int(time.Since(t0)/time.Millisecond), "tls memory in use", int(tm.Stats().InUse))
	}
	println("https-get done")
	for {
		time.Sleep(time.Hour)
	}
}

var connected = make(chan struct{}, 1)

func must(what string, err error) {
	if err != nil {
		println(what+":", err.Error())
		for {
			time.Sleep(time.Hour)
		}
	}
}
