// Command tls-get gets https://www.google.com/ with TLS 1.3. It takes the time
// from pool.ntp.org for the validity check, and it trusts the P-256 keys of
// the Google Trust Services intermediate CAs WE1 and WE2 (valid until 2029).
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
	socketMem [16 << 10]byte
	tlsMem    [40 << 10]byte
	page      [1024]byte
)

// The keys of Google Trust Services WE1 and WE2.
var roots = []tls.PublicKey{
	key("6fcd3afe6757474c21038540c2475dbb58470f40c15c1785c61937e7d57ced86", "4b9b81d9d71a13a50a03f898c4c6e89eff10598f2c2698f5e62625bb0f02fa56"),
	key("357e1ff214ed907de19e2a344386c1d596e82770df9e04cba9ca86790b084d46", "8ac274a4bbd9bfeefd23d738f34bef5417e1bee7ca5525a80c30ac2d5d4ea151"),
}

func key(x, y string) tls.PublicKey {
	var k tls.PublicKey
	hex.Decode(k.X[:], []byte(x))
	hex.Decode(k.Y[:], []byte(y))
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
	raw, err := stack.DialTCP(addr, 443, 4096, 5*time.Second)
	must("dial", err)
	tm, err := arena.NewHeap(tlsMem[:])
	must("tls memory", err)
	cfg := &tls.Config{ServerName: host, Time: now.Add(time.Since(boot)), Roots: roots}
	c := tls.Client(raw, tm, rng{}, cfg)
	t0 := time.Now()
	must("handshake", c.Handshake())
	println("handshake ms", int(time.Since(t0)/time.Millisecond), "tls memory peak", tm.Stats().Peak)

	_, err = c.Write([]byte("GET / HTTP/1.1\r\nHost: " + host + "\r\nConnection: close\r\n\r\n"))
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
	println("tls-get done")
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
