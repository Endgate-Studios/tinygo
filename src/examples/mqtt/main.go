// Command mqtt connects an ESP32-S3 to an MQTT broker. It subscribes to
// tinygo-demo/ID/cmd, publishes a message there to check the round trip
// through the broker, and then publishes its uptime to tinygo-demo/ID/uptime
// each 10 seconds. ID is random at each start. The broker is
// broker.hivemq.com by default, and anyone can read its topics. The SSID,
// the password and another broker come from the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..." -X "main.broker=..."' ./src/examples/mqtt
package main

import (
	"device/esp"
	"device/esp/radio"
	"runtime/memory/arena"
	"strconv"
	"time"

	"tinygo/net/ip"
	"tinygo/net/mqtt"
)

var (
	ssid, password string
	broker         = "broker.hivemq.com"
)

var (
	socketMem [16 << 10]byte
	rx, tx    [512]byte
)

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

	// The RNG is a true random source while Wi-Fi runs (ESP32-S3 Technical
	// Reference Manual, chapter Random Number Generator).
	id := strconv.FormatUint(uint64(esp.RNG.DATA.Get()), 16)
	prefix := "tinygo-demo/" + id
	addr, err := stack.LookupHost(broker, 5*time.Second)
	must("lookup", err)
	conn, err := stack.DialTCP(addr, 1883, 2048, 5*time.Second)
	must("dial", err)

	start := time.Now()
	c := mqtt.New(conn, rx[:], tx[:], func(m *mqtt.Message) {
		println("message", string(m.Topic), string(m.Payload))
	})
	must("mqtt connect", c.Connect(mqtt.Config{ClientID: "tinygo-" + id, KeepAlive: 30 * time.Second}))
	println("connected to", broker, "as", prefix)
	must("subscribe", c.Subscribe(prefix+"/cmd", 1))
	must("publish", c.Publish(prefix+"/cmd", []byte("hello from TinyGo"), 1, false))

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
