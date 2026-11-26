// Command espnow sends and receives ESP-NOW messages on Wi-Fi channel 1, with
// no access point. Flash it to two or more boards. Each board sends a
// broadcast each second, prints the messages that it receives, and sends a
// unicast answer to each new board that it hears:
//
//	tinygo flash -target esp32s3-psram-octal ./src/examples/espnow
package main

import (
	"device/esp/radio"
	"strconv"
	"time"
)

const channel = 1

// heard is a new board, for the main goroutine.
var heard = make(chan [6]byte, 4)

func main() {
	time.Sleep(2 * time.Second)
	must("init", radio.Init())
	must("mode", radio.SetMode(radio.ModeStation))
	must("start", radio.Start())
	must("channel", radio.SetChannel(channel))
	must("espnow", radio.ESPNowInit())
	must("broadcast peer", radio.ESPNowAddPeer(radio.ESPNowPeer{MAC: radio.ESPNowBroadcast}))

	me := radio.MAC()
	println("espnow on channel", channel, "as", hexMAC(me[:]))
	peers := map[[6]byte]bool{}
	radio.HandleESPNow(func(m *radio.ESPNowMessage) {
		println("from", hexMAC(m.Src[:]), "rssi", m.RSSI, ":", string(m.Data))
		select {
		case heard <- m.Src:
		default:
		}
	})

	var buf [64]byte
	for n := 1; ; n++ {
		msg := append(append(buf[:0], "hello "...), strconv.Itoa(n)...)
		if err := radio.ESPNowSend(radio.ESPNowBroadcast, msg); err != nil {
			println("broadcast:", err.Error())
		}
		for wait := time.After(time.Second); ; {
			select {
			case mac := <-heard:
				if peers[mac] {
					continue
				}
				peers[mac] = true
				must("peer", radio.ESPNowAddPeer(radio.ESPNowPeer{MAC: mac}))
				err := radio.ESPNowSend(mac, append(buf[:0], "hello peer"...))
				if err != nil {
					println("unicast to", hexMAC(mac[:])+":", err.Error())
				} else {
					println("unicast to", hexMAC(mac[:]), "acknowledged")
				}
				continue
			case <-wait:
			}
			break
		}
	}
}

// hexMAC formats a MAC address.
func hexMAC(b []byte) string {
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
