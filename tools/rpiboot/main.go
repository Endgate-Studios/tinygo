// Command rpiboot boots a Raspberry Pi Zero 2 W over USB with a TinyGo kernel image.
// It puts the image next to the Pi's boot firmware and config.txt, and serves them
// with Raspberry Pi's rpiboot (github.com/raspberrypi/usbboot).
//
// RPI_BOOT_FILES is the folder with bootcode.bin, start.elf and fixup.dat from
// github.com/raspberrypi/firmware/tree/master/boot. RPIBOOT is rpiboot's path, when it's
// not on PATH or in its installer's folder.
//
// RPI_FIRMWARE_FILES, if set, is a folder whose files go in a bundle that config.txt loads
// at 0x0F000000, where machine.FirmwareFile finds them: the Wi-Fi chip's firmware,
// CLM blob and NVRAM text from github.com/RPi-Distro/firmware-nonfree.
package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

const bundleAddress = "0x0f000000"

const configTxt = `arm_64bit=1
kernel=kernel8.img
core_freq=250
core_freq_min=250
enable_uart=1
gpu_mem=64
disable_splash=1
`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: rpiboot kernel.bin")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "rpiboot:", err)
		os.Exit(1)
	}
}

func run(kernel string) error {
	boot := os.Getenv("RPI_BOOT_FILES")
	if boot == "" {
		return fmt.Errorf("set RPI_BOOT_FILES to a folder with bootcode.bin, start.elf and fixup.dat")
	}
	tool, err := findRpiboot()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "rpiboot")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"bootcode.bin", "start.elf", "fixup.dat"} {
		if err := copyFile(filepath.Join(boot, name), filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	if err := copyFile(kernel, filepath.Join(dir, "kernel8.img")); err != nil {
		return err
	}
	config := configTxt
	if fw := os.Getenv("RPI_FIRMWARE_FILES"); fw != "" {
		if err := writeBundle(fw, filepath.Join(dir, "firmware.bin")); err != nil {
			return err
		}
		config += "initramfs firmware.bin " + bundleAddress + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "config.txt"), []byte(config), 0o644); err != nil {
		return err
	}
	cmd := exec.Command(tool, "-d", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func findRpiboot() (string, error) {
	if p := os.Getenv("RPIBOOT"); p != "" {
		return p, nil
	}
	if p, err := exec.LookPath("rpiboot"); err == nil {
		return p, nil
	}
	for _, p := range []string{
		`C:\Program Files (x86)\Raspberry Pi\rpiboot.exe`,
		`C:\Program Files\Raspberry Pi\rpiboot.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("rpiboot not found: install it from github.com/raspberrypi/usbboot or set RPIBOOT")
}

// writeBundle packs the files of dir as "TGFW", a count, 64 byte entries of a 56 byte
// name, offset and size, then the files at 16 byte boundaries.
func writeBundle(dir, out string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && len(e.Name()) < 56 {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	head := make([]byte, 8+64*len(names))
	copy(head, "TGFW")
	binary.LittleEndian.PutUint32(head[4:], uint32(len(names)))
	var body []byte
	offset := (len(head) + 15) &^ 15
	for i, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		e := head[8+64*i:]
		copy(e[:56], name)
		binary.LittleEndian.PutUint32(e[56:], uint32(offset+len(body)))
		binary.LittleEndian.PutUint32(e[60:], uint32(len(data)))
		body = append(body, data...)
		for len(body)%16 != 0 {
			body = append(body, 0)
		}
	}
	file := append(head, make([]byte, offset-len(head))...)
	file = append(file, body...)
	return os.WriteFile(out, file, 0o644)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
