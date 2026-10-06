// Command mqtt-tls connects an ESP32-S3 to the MQTT broker broker.emqx.io
// with TLS 1.3 on port 8883. It takes the time from pool.ntp.org for the
// validity check, and it trusts the RSA key of the root CA DigiCert Global
// Root G2. Then it subscribes to tinygo-demo/ID/cmd, publishes a message
// there, and publishes its uptime to tinygo-demo/ID/uptime each 10 seconds.
// ID is random at each start, and anyone can read the topics. The SSID and
// the password come from the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/mqtt-tls
package main

import (
	"device/esp"
	"device/esp/radio"
	"encoding/hex"
	"runtime/memory/arena"
	"strconv"
	"time"

	"tinygo/net/ip"
	"tinygo/net/mqtt"
	"tinygo/net/tls"
)

var ssid, password string

const broker = "broker.emqx.io"

var (
	socketMem [16 << 10]byte
	tlsMem    [40 << 10]byte
	rx, tx    [512]byte
	g2N       [256]byte
)

// g2Modulus is the RSA modulus of DigiCert Global Root G2. Its exponent is
// 65537.
const g2Modulus = "bb37cd34dc7b6bc9b26890ad4a75ff46ba210a088df51954c9fb88dbf3aef23a" +
	"89913c7ae6ab061a6bcfac2de85e092444ba629a7ed6a3a87ee054752005ac50" +
	"b79c631a6c30dcda1f19b1d71edefdd7e0cb948337aeec1f434edd7b2cd2bd2e" +
	"a52fe4a9b8ad3ad499a4b625e99b6b00609260ff4f214918f76790ab61069c8f" +
	"f2bae9b4e992326bb5f357e85d1bcd8c1dab95049549f3352d96e3496ddd77e3" +
	"fb494bb4ac5507a98f95b3b423bb4c6d45f0f6a9b29530b4fd4c558c274a5714" +
	"7c829dcd7392d3164a060c8c50d18f1e09be17a1e621cafd83e510bc83a50ac4" +
	"6728f67314143d4676c387148921344daf0f450ca649a1babb9cc5b133832985"

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
	go keepConnected()
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

	addr, err := stack.LookupHost(broker, 5*time.Second)
	must("lookup", err)
	raw, err := stack.DialTCP(addr, 8883, 4096, 5*time.Second)
	must("dial", err)
	tm, err := arena.NewHeap(tlsMem[:])
	must("tls memory", err)
	hex.Decode(g2N[:], []byte(g2Modulus))
	root := tls.PublicKey{}
	root.RSA.N, root.RSA.E = g2N[:], 65537
	cfg := &tls.Config{ServerName: broker, Time: now.Add(time.Since(boot)), Roots: []tls.PublicKey{root}}
	conn := tls.Client(raw, tm, rng{}, cfg)
	t0 := time.Now()
	must("tls handshake", conn.Handshake())
	println("tls handshake ms", int(time.Since(t0)/time.Millisecond))

	id := strconv.FormatUint(uint64(esp.RNG.DATA.Get()), 16)
	prefix := "tinygo-demo/" + id
	start := time.Now()
	c := mqtt.New(conn, rx[:], tx[:], func(m *mqtt.Message) {
		println("message", string(m.Topic), string(m.Payload))
	})
	must("mqtt connect", c.Connect(mqtt.Config{ClientID: "tinygo-" + id, KeepAlive: 30 * time.Second}))
	println("connected to", broker, "with TLS as", prefix)
	must("subscribe", c.Subscribe(prefix+"/cmd", 1))
	must("publish", c.Publish(prefix+"/cmd", []byte("hello over TLS"), 1, false))

	next := time.Now()
	var buf [24]byte
	for {
		if time.Now().After(next) {
			up := strconv.AppendInt(buf[:0], int64(time.Since(start)/time.Second), 10)
			must("publish", c.Publish(prefix+"/uptime", up, 0, false))
			println("published uptime", string(up))
			next = next.Add(10 * time.Second)
		}
		must("poll", c.Poll(time.Until(next)))
	}
}

// connected gets a value at each connection of the station.
var connected = make(chan struct{}, 1)

// keepConnected reads the radio events. After a disconnect, it waits one
// second and connects again.
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
