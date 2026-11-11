// Joins a Wi-Fi network on a Raspberry Pi Zero 2 W, gets an address with DHCP, answers ping
// and echoes TCP on port 7. The log is on the USB console.
//
//	RPI_BOOT_FILES=... RPI_FIRMWARE_FILES=... tinygo flash -target=rpi-zero2w \
//	    -ldflags="-X main.ssid=MyNetwork -X main.pass=secret" ./src/examples/rpi-wifi
//
// RPI_FIRMWARE_FILES holds brcmfmac43436-sdio.bin, .txt and .clm_blob, and the 43436s
// files, from github.com/RPi-Distro/firmware-nonfree debian/added-firmware/brcm.
package main

import (
	"device/cyw43"
	"machine"
	"runtime/memory/arena"
	"time"

	"tinygo/net/ip"
)

var ssid, pass string

var socketMem [256 * 1024]byte

func main() {
	time.Sleep(3 * time.Second)
	println("rpi-wifi: start")
	if err := run(); err != nil {
		println("rpi-wifi:", err.Error())
	}
	for {
		time.Sleep(time.Hour)
	}
}

func run() error {
	if rev, err := machine.BoardRevision(); err == nil {
		println("board revision", rev)
	}
	if err := machine.SDIO0.Configure(machine.SDIOConfig{PowerPin: machine.WIFI_ON}); err != nil {
		return err
	}
	dev := cyw43.New(machine.SDIO0)
	if err := dev.Init(); err != nil {
		return err
	}
	chip, rev := dev.Chip()
	println("chip", chip, "rev", rev)

	// Revision 1 is the 43436s, as Linux brcmfmac sdio.c's firmware table has it.
	name := "brcmfmac43436-sdio"
	if rev == 1 {
		name = "brcmfmac43436s-sdio"
	}
	fw := machine.FirmwareFile(name + ".bin")
	nvram := machine.FirmwareFile(name + ".txt")
	clm := machine.FirmwareFile("brcmfmac43436-sdio.clm_blob")
	if fw == nil || nvram == nil {
		println("no", name, "files in the firmware bundle: set RPI_FIRMWARE_FILES")
		return cyw43.ErrNoFirmware
	}
	if err := dev.Start(fw, nvram, clm); err != nil {
		return err
	}
	println("firmware", dev.Version())
	mac := dev.MAC()
	println("mac", hex(mac[:]))

	if ssid == "" {
		println("no network: build with -ldflags=\"-X main.ssid=... -X main.pass=...\"")
		return nil
	}
	println("joining", ssid)
	if err := dev.Join(ssid, pass, 20*time.Second); err != nil {
		return err
	}
	println("joined")

	mem, err := arena.NewHeap(socketMem[:])
	if err != nil {
		return err
	}
	stack := ip.New(dev, mem)
	if serial, err := machine.BoardSerial(); err == nil {
		stack.Seed(uint32(serial) ^ uint32(time.Now().UnixNano()))
	}
	dev.HandleFrames(stack.Input)
	if err := stack.DHCP(15 * time.Second); err != nil {
		return err
	}
	c := stack.Config()
	println("address", c.Addr.String(), "gateway", c.Gateway.String())

	l, err := stack.ListenTCP(7, 2048)
	if err != nil {
		return err
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go echo(conn)
	}
}

func echo(conn *ip.TCPConn) {
	defer conn.Close()
	var buf [512]byte
	for {
		n, err := conn.Read(buf[:])
		if err != nil {
			return
		}
		if _, err := conn.Write(buf[:n]); err != nil {
			return
		}
	}
}

func hex(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*3)
	for i, c := range b {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, digits[c>>4], digits[c&15])
	}
	return string(out)
}
