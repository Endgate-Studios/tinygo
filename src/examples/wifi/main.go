// Command wifi connects an ESP32-S3 to a WPA2 network, gets an address with
// DHCP, gets a page with HTTP, answers ping and runs a UDP and a TCP echo
// server on port 7. The SSID and the password come from the link:
//
//	tinygo flash -target esp32s3-psram-octal -ldflags='-X "main.ssid=..." -X "main.password=..."' ./src/examples/wifi
package main

import (
	"device/esp/radio"
	"io"
	"runtime/memory/arena"
	"time"
	"tinygo/net/ip"
)

var ssid, password string

// httpHost and httpPort are the server of the HTTP test. Set them with -X.
var httpHost, httpPort = "example.com", "80"

// socketMem holds the socket rings: 2 KB for UDP and 4 KB for each TCP
// connection.
var socketMem [16 << 10]byte

func main() {
	time.Sleep(2 * time.Second)
	if ssid == "" {
		println("set main.ssid and main.password with -ldflags -X")
		return
	}
	must("init", radio.Init())
	must("mode", radio.SetMode(radio.ModeStation))
	must("start", radio.Start())
	must("connect", radio.Connect(ssid, password))
	waitConnected()

	mem, err := arena.NewHeap(socketMem[:])
	must("socket memory", err)
	stack := ip.New(radio.Station{}, mem)
	radio.HandleFrames(stack.Input)

	must("dhcp", stack.DHCP(30*time.Second))
	c := stack.Config()
	println("address", c.Addr.String(), "gateway", c.Gateway.String(), "dns", c.DNS.String(), "lease", int(stack.Lease().Seconds()), "s")

	httpGet(stack, httpHost, httpPort)

	echo, err := stack.ListenUDP(7, 2048)
	must("listen", err)
	tcpEcho, err := stack.ListenTCP(7, 2048)
	must("listen tcp", err)
	go serveTCP(tcpEcho)
	go stats(stack)
	buf := make([]byte, 1472)
	for {
		n, from, port, err := echo.ReadFrom(buf)
		if err != nil {
			println("read:", err.Error())
			return
		}
		if err := echo.WriteTo(buf[:n], from, port); err != nil {
			println("write:", err.Error())
		}
	}
}

// httpGet looks up host, sends an HTTP/1.0 GET and prints the status line.
func httpGet(stack *ip.Stack, host, portText string) {
	port := uint16(0)
	for _, ch := range portText {
		port = port*10 + uint16(ch-'0')
	}
	addr, err := stack.LookupHost(host, 5*time.Second)
	if err != nil {
		println("lookup", host+":", err.Error())
		return
	}
	println("lookup", host, addr.String())
	c, err := stack.DialTCP(addr, port, 0, 5*time.Second)
	if err != nil {
		println("dial:", err.Error())
		return
	}
	if _, err := c.Write([]byte("GET / HTTP/1.0\r\nHost: " + host + "\r\n\r\n")); err != nil {
		println("write:", err.Error())
	}
	var buf [512]byte
	total, line := 0, ""
	for {
		n, err := c.Read(buf[:])
		if err != nil && err != io.EOF {
			println("read:", err.Error())
		}
		if total == 0 && n > 0 {
			end := 0
			for end < n && buf[end] != '\r' {
				end++
			}
			line = string(buf[:end])
		}
		total += n
		if err != nil {
			break
		}
	}
	c.Close()
	println("http", line, "|", total, "bytes")
}

// serveTCP echoes one connection at a time.
func serveTCP(l *ip.TCPListener) {
	buf := make([]byte, 1460)
	for {
		c, err := l.Accept()
		if err != nil {
			println("accept:", err.Error())
			return
		}
		total := 0
		for {
			n, err := c.Read(buf)
			if err != nil {
				break
			}
			if _, err := c.Write(buf[:n]); err != nil {
				break
			}
			total += n
		}
		c.Close()
		addr, port := c.RemoteAddr()
		println("tcp echo", total, "bytes for", addr.String(), port)
	}
}

func waitConnected() {
	for e := range radio.Events() {
		switch e.ID {
		case radio.EventStaConnected:
			println("connected to", ssid)
			return
		case radio.EventStaDisconnected:
			println("disconnected")
		}
	}
}

func stats(stack *ip.Stack) {
	for {
		time.Sleep(10 * time.Second)
		s := stack.Stats()
		m := radio.MemoryStats()
		println("frames", s.Frames, "echo replies", s.EchoReplies, "arp replies", s.ARPReplies,
			"udp", s.UDPDelivered, "tcp", s.TCPSegments, "dropped", s.Dropped, s.UDPDropped, radio.FramesDropped(), "| blob heap", m.InUse, "peak", m.Peak)
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
