// Command tls-get gets https://www.google.com/ with TLS 1.3. It takes the time
// from pool.ntp.org for the validity check, and it trusts the P-384 key of
// the Google Trust Services root CA GTS Root R4 (valid until 2036). A second
// request resumes the TLS session with the ticket of the first.
// The SSID and the password come from the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/tls-get
package main

import (
	"device/esp"
	"device/esp/radio"
	"encoding/hex"
	"runtime/memory/arena"
	"time"

	"tinygo/net/ip"
	"tinygo/net/tls"
)

var ssid, password string

const host = "www.google.com"

var (
	socketMem [32 << 10]byte
	tlsMem    [40 << 10]byte
	page      [1024]byte
)

// The key of GTS Root R4, x and y.
var roots = []tls.PublicKey{key384(
	"f37473a7688b60ae43b835c581307b4b499dfbc161cee6de46bd6bd5611835ae40dd73f78991305aeb3cee857ca24076" +
		"3ba9c6b847d82ae792916a73e9b172399f299fa298d35f5e5886650fa1846506d1dc8bc9c773c88c6a2fe5c4abd11d8a")}

func key384(xy string) tls.PublicKey {
	k := tls.PublicKey{P384: make([]byte, 96)}
	hex.Decode(k.P384, []byte(xy))
	return k
}

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
	println("time", now.Format(time.RFC3339))
	boot := time.Now()

	addr, err := stack.LookupHost(host, 5*time.Second)
	must("lookup", err)
	// The second request resumes with the ticket of the first one.
	var session tls.Session
	for i := 0; i < 2; i++ {
		get(stack, addr, &tls.Config{ServerName: host, Time: now.Add(time.Since(boot)), Roots: roots, Session: &session})
	}
	println("tls-get done")
	for {
		time.Sleep(time.Hour)
	}
}

func get(stack *ip.Stack, addr ip.Addr, cfg *tls.Config) {
	raw, err := stack.DialTCP(addr, 443, 4096, 5*time.Second)
	must("dial", err)
	tm, err := arena.NewHeap(tlsMem[:])
	must("tls memory", err)
	c := tls.Client(raw, tm, rng{}, cfg)
	t0 := time.Now()
	must("handshake", c.Handshake())
	println("handshake ms", int(time.Since(t0)/time.Millisecond), "resumed", c.Resumed(), "tls memory peak", tm.Stats().Peak)

	_, err = c.Write([]byte("GET / HTTP/1.1\r\nHost: " + cfg.ServerName + "\r\nConnection: close\r\n\r\n"))
	must("write", err)
	total, first := 0, true
	for {
		n, err := c.Read(page[:])
		if first && n > 0 {
			end := 0
			for end < n && page[end] != '\r' {
				end++
			}
			println("status", string(page[:end]))
			first = false
		}
		total += n
		if err != nil {
			println("done:", total, "bytes,", err.Error())
			break
		}
	}
	c.Close()
	raw.Close()
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
