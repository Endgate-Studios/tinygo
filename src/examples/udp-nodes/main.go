// Command udp-nodes finds the other nodes on the Wi-Fi network with UDP
// broadcast. Each node sends a message to the subnet broadcast address on port
// 4210 each 2 seconds, and keeps a table of the nodes that it hears. Any
// device can join, for example a PC with a few lines of Python. Each node also
// answers mDNS for tinygo-XXXXXX.local, with the end of its MAC address. The
// SSID and the password come from the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/udp-nodes
package main

import (
	"device/esp/radio"
	"runtime/memory/arena"
	"strconv"
	"time"

	"tinygo/net/ip"
)

var ssid, password string

const (
	port     = 4210
	interval = 2 * time.Second
	timeout  = 3 * interval // a node that is silent this long has left
	prefix   = "tinygo-node "
)

var socketMem [8 << 10]byte

type node struct {
	addr ip.Addr
	name [32]byte
	n    int
	seen time.Time
}

var nodes [8]node

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
	cfg := stack.Config()
	println("address", cfg.Addr.String())

	mac := radio.MAC()
	name := string(appendHex([]byte("tinygo-"), mac[3:]))
	go func() { must("mdns", stack.ServeMDNS(name)) }()
	println("mdns name", name+".local")

	c, err := stack.ListenUDP(port, 1024)
	must("listen", err)
	go announce(c, cfg)

	var buf [128]byte
	for {
		c.SetReadDeadline(time.Now().Add(time.Second))
		n, from, _, err := c.ReadFrom(buf[:])
		now := time.Now()
		if err == nil && from != stack.Config().Addr {
			heard(from, buf[:n], now)
		}
		for i := range nodes {
			if !nodes[i].addr.IsZero() && now.Sub(nodes[i].seen) > timeout {
				println("node left", nodes[i].addr.String(), string(nodes[i].name[:nodes[i].n]))
				nodes[i] = node{}
			}
		}
	}
}

// announce sends this node to the subnet broadcast address at each interval.
func announce(c *ip.UDPConn, cfg ip.Config) {
	var bcast ip.Addr
	for i := range bcast {
		bcast[i] = cfg.Addr[i] | ^cfg.Mask[i]
	}
	mac := radio.MAC()
	var buf [64]byte
	start := time.Now()
	for {
		msg := append(buf[:0], prefix...)
		msg = appendHex(msg, mac[3:])
		msg = append(msg, " up "...)
		msg = strconv.AppendInt(msg, int64(time.Since(start)/time.Second), 10)
		if err := c.WriteTo(msg, bcast, port); err != nil {
			println("announce:", err.Error())
		}
		time.Sleep(interval)
	}
}

// heard updates the table with a message from another node.
func heard(from ip.Addr, msg []byte, now time.Time) {
	if len(msg) <= len(prefix) || string(msg[:len(prefix)]) != prefix {
		return
	}
	name := msg[len(prefix):]
	if i := index(name, ' '); i >= 0 {
		name = name[:i]
	}
	free := -1
	for i := range nodes {
		switch {
		case nodes[i].addr == from:
			nodes[i].seen = now
			return
		case nodes[i].addr.IsZero() && free < 0:
			free = i
		}
	}
	if free < 0 {
		return // the table is full
	}
	nd := &nodes[free]
	nd.addr, nd.seen = from, now
	nd.n = copy(nd.name[:], name)
	println("node joined", from.String(), string(nd.name[:nd.n]))
}

func index(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func appendHex(b, v []byte) []byte {
	const digits = "0123456789abcdef"
	for _, x := range v {
		b = append(b, digits[x>>4], digits[x&15])
	}
	return b
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
