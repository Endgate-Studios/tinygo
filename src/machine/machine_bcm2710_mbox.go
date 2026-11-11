//go:build bcm2710

package machine

import (
	"device/arm64"
	"device/bcm2710"
	"errors"
	"sync"
	"unsafe"
)

// The VideoCore's property interface, from the Raspberry Pi firmware wiki,
// "Mailbox property interface". Channel 8 is ARM to VC.
const mboxChannelProperty = 8

// Property tags.
const (
	tagGetBoardRevision = 0x00010002
	tagGetBoardMAC      = 0x00010003
	tagGetBoardSerial   = 0x00010004
	tagSetPowerState    = 0x00028001
	tagGetClockRate     = 0x00030002
)

// Power domain and clock ids of the property interface.
const (
	PowerSD  = 0
	PowerUSB = 3

	ClockEMMC = 1
	ClockCore = 4
	ClockARM  = 3
)

var (
	mboxLock sync.Mutex
	mboxBuf  = (*[64]uint32)(dmaAlloc(256, 16))

	errMailbox = errors.New("mailbox: the VideoCore didn't answer the request")
)

// mboxCall sends one property tag with words of request and returns its reply words.
// The buffer is in the uncached DMA region, so the VideoCore sees it without cache upkeep.
func mboxCall(tag uint32, words int, req ...uint32) ([]uint32, error) {
	mboxLock.Lock()
	defer mboxLock.Unlock()
	b := mboxBuf
	n := 5 + words + 1
	b[0] = uint32(n * 4)
	b[1] = 0
	b[2] = tag
	b[3] = uint32(words * 4)
	b[4] = 0
	for i := 0; i < words; i++ {
		b[5+i] = 0
		if i < len(req) {
			b[5+i] = req[i]
		}
	}
	b[5+words] = 0

	addr := uint32(uintptr(unsafe.Pointer(b))) | bcm2710.BusUncached
	arm64.Asm("dsb sy")
	for bcm2710.MBOX.STATUS1.Get()&bcm2710.MBOX_FULL != 0 {
	}
	bcm2710.MBOX.WRITE.Set(addr | mboxChannelProperty)
	for {
		for bcm2710.MBOX.STATUS.Get()&bcm2710.MBOX_EMPTY != 0 {
		}
		if bcm2710.MBOX.READ.Get() == addr|mboxChannelProperty {
			break
		}
	}
	arm64.Asm("dsb sy")
	if b[1] != 0x80000000 || b[4]&0x80000000 == 0 {
		return nil, errMailbox
	}
	out := make([]uint32, words)
	copy(out, b[5:5+words])
	return out, nil
}

// SetPowerState turns a VideoCore power domain on or off and waits for it.
func SetPowerState(device uint32, on bool) error {
	state := uint32(2) // wait
	if on {
		state |= 1
	}
	r, err := mboxCall(tagSetPowerState, 2, device, state)
	if err != nil {
		return err
	}
	if r[1]&2 != 0 {
		return errors.New("mailbox: no such power domain")
	}
	return nil
}

// ClockRate returns a clock's rate in Hz.
func ClockRate(clock uint32) (uint32, error) {
	r, err := mboxCall(tagGetClockRate, 2, clock)
	if err != nil {
		return 0, err
	}
	return r[1], nil
}

// BoardRevision returns the revision code, 0x902120 for a Zero 2 W.
func BoardRevision() (uint32, error) {
	r, err := mboxCall(tagGetBoardRevision, 1)
	if err != nil {
		return 0, err
	}
	return r[0], nil
}

// BoardMAC returns the MAC address the firmware gives the board.
func BoardMAC() ([6]byte, error) {
	var mac [6]byte
	r, err := mboxCall(tagGetBoardMAC, 2)
	if err != nil {
		return mac, err
	}
	mac = [6]byte{byte(r[0]), byte(r[0] >> 8), byte(r[0] >> 16), byte(r[0] >> 24), byte(r[1]), byte(r[1] >> 8)}
	return mac, nil
}

// BoardSerial returns the board's serial number.
func BoardSerial() (uint64, error) {
	r, err := mboxCall(tagGetBoardSerial, 2)
	if err != nil {
		return 0, err
	}
	return uint64(r[1])<<32 | uint64(r[0]), nil
}

// CPUFrequency returns the ARM clock in Hz.
func CPUFrequency() uint32 {
	f, _ := ClockRate(ClockARM)
	return f
}

var dmaNext uintptr = bcm2710.DMAStart

// dmaAlloc gives memory from the uncached DMA region. It's never freed.
func dmaAlloc(size, align uintptr) unsafe.Pointer {
	p := (dmaNext + align - 1) &^ (align - 1)
	if p+size > bcm2710.DMAEnd {
		panic("machine: DMA region full")
	}
	dmaNext = p + size
	return unsafe.Pointer(p)
}

// FirmwareFile returns a file that tools/rpiboot packed in the firmware bundle, or nil.
// The bundle is "TGFW", a count, then 64 byte entries of a 56 byte name, offset and size.
func FirmwareFile(name string) []byte {
	base := unsafe.Pointer(uintptr(bcm2710.FirmwareBundle))
	head := unsafe.Slice((*byte)(base), 8)
	if string(head[:4]) != "TGFW" {
		return nil
	}
	count := le32(head[4:])
	if count > 64 {
		return nil
	}
	entries := unsafe.Slice((*byte)(unsafe.Add(base, 8)), int(count)*64)
	for i := 0; i < int(count); i++ {
		e := entries[i*64 : i*64+64]
		n := 0
		for n < 56 && e[n] != 0 {
			n++
		}
		if string(e[:n]) != name {
			continue
		}
		off := le32(e[56:])
		size := le32(e[60:])
		if uintptr(off)+uintptr(size) > bcm2710.DMAStart-bcm2710.FirmwareBundle {
			return nil
		}
		return unsafe.Slice((*byte)(unsafe.Add(base, uintptr(off))), int(size))
	}
	return nil
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
