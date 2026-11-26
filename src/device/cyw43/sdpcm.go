package cyw43

import (
	"time"
)

// SDPCM frames: a 4 byte length tag and an 8 byte header (brcmfmac sdio.c, SDPCM_HDRLEN).
const (
	sdpcmHeaderLen = 12
	chanControl    = 0
	chanEvent      = 1
	chanData       = 2

	bcdcHeaderLen = 16 // struct brcmf_proto_bcdc_dcmd
	bdcHeaderLen  = 4  // struct brcmf_proto_bcdc_header
	bcdcSet       = 0x02
	bcdcIDShift   = 16
	bdcVersion2   = 2 << 4

	maxFrame = 1600
)

// Firmware commands (brcmfmac fwil.h, BRCMF_C_*).
const (
	cmdUp        = 2
	cmdDown      = 3
	cmdSetInfra  = 20
	cmdSetAuth   = 22
	cmdSetSSID   = 26
	cmdDisassoc  = 52
	cmdSetAntdiv = 64
	cmdSetPM     = 86
	cmdSetGMode  = 110
	cmdSetWsec   = 134
	cmdSetBand   = 142
	cmdSetWPA    = 165
	cmdGetVar    = 262
	cmdSetVar    = 263
	cmdSetPMK    = 268
)

// frameQueue holds received Ethernet frames until the poller hands them on without the bus lock.
type frameQueue struct {
	buf  [16][maxFrame]byte
	n    [16]int
	head int
	used int
	lost uint32
}

func (q *frameQueue) put(frame []byte) {
	if q.used == len(q.buf) || len(frame) > maxFrame {
		q.lost++
		return
	}
	i := (q.head + q.used) % len(q.buf)
	q.n[i] = copy(q.buf[i][:], frame)
	q.used++
}

func (q *frameQueue) get(out []byte) (int, bool) {
	if q.used == 0 {
		return 0, false
	}
	n := copy(out, q.buf[q.head][:q.n[q.head]])
	q.head = (q.head + 1) % len(q.buf)
	q.used--
	return n, true
}

// send writes one SDPCM frame on channel ch with payload, padded for the bus.
func (d *Device) send(ch byte, payload []byte) error {
	total := sdpcmHeaderLen + len(payload)
	if total > len(d.txBuf) {
		return ErrFrameTooBig
	}
	for i := 0; ; i++ {
		if d.txMax != d.txSeq && (d.txMax-d.txSeq)&0x80 == 0 {
			break
		}
		// No credit: read what the chip sent, which carries new credit.
		d.receive()
		if i > 200 {
			return ErrTimeout
		}
		if i > 2 {
			time.Sleep(time.Millisecond)
		}
	}
	b := d.txBuf[:]
	put16(b[0:], uint16(total))
	put16(b[2:], ^uint16(total))
	b[4] = d.txSeq
	b[5] = ch
	b[6] = 0
	b[7] = sdpcmHeaderLen
	b[8], b[9], b[10], b[11] = 0, 0, 0, 0
	copy(b[sdpcmHeaderLen:], payload)
	n := (total + 3) &^ 3
	if n > blockSizeWLAN {
		n = (n + blockSizeWLAN - 1) &^ (blockSizeWLAN - 1)
	}
	for i := total; i < n; i++ {
		b[i] = 0
	}
	d.txSeq++
	return d.bus.WriteData(fnWLAN, 0, b[:n], false)
}

// receive reads every frame the chip has waiting. Data frames go to the queue, events to
// the event state, and a control reply to ctlBuf.
func (d *Device) receive() bool {
	status, err := d.read32(sdIntStatus)
	if err != nil {
		return false
	}
	if status != 0 {
		d.write32(sdIntStatus, status)
	}
	if status&intHostMailbox != 0 {
		d.read32(sdToHostMbData)
		d.write32(sdToSBMailbox, smbIntAck)
	}
	got := false
	for i := 0; i < 32; i++ {
		if !d.readFrame() {
			break
		}
		got = true
	}
	return got
}

// readFrame reads one frame: 64 bytes first, then the rest, as brcmfmac's first read does.
func (d *Device) readFrame() bool {
	b := d.rxBuf[:]
	if d.bus.ReadData(fnWLAN, 0, b[:64], false) != nil {
		return false
	}
	n := int(le16(b[0:]))
	if n == 0 || n < sdpcmHeaderLen || le16(b[2:]) != ^uint16(n) {
		return false
	}
	if n > len(b) {
		return false
	}
	if n > 64 {
		rest := (n - 64 + 3) &^ 3
		if 64+rest > len(b) {
			rest = len(b) - 64
		}
		if d.bus.ReadData(fnWLAN, 0, b[64:64+rest], false) != nil {
			return false
		}
	}
	credit := b[9]
	if credit-d.txSeq > 0x40 {
		credit = d.txSeq + 2
	}
	d.txMax = credit
	off := int(b[7])
	if off < sdpcmHeaderLen || off > n {
		return true
	}
	payload := b[off:n]
	switch b[5] & 0x0f {
	case chanControl:
		d.handleControl(payload)
	case chanEvent:
		if p := bdcPayload(payload); p != nil {
			d.events.handle(p)
		}
	case chanData:
		if p := bdcPayload(payload); p != nil {
			d.rxq.put(p)
		}
	}
	return true
}

// bdcPayload skips the BDC header and its data offset in words.
func bdcPayload(p []byte) []byte {
	if len(p) < bdcHeaderLen {
		return nil
	}
	skip := bdcHeaderLen + int(p[3])*4
	if skip > len(p) {
		return nil
	}
	return p[skip:]
}

func (d *Device) handleControl(p []byte) {
	if len(p) < bcdcHeaderLen {
		return
	}
	flags := le32(p[8:])
	if uint16(flags>>bcdcIDShift) != d.ctlID {
		return
	}
	d.ctlErr = le32(p[12:]) != 0
	if d.ctlBuf != nil {
		d.ctlBuf = d.ctlBuf[:copy(d.ctlBuf[:cap(d.ctlBuf)], p[bcdcHeaderLen:])]
	}
	d.ctlOK = true
}

// ioctl sends a firmware command and waits for its reply. For a get, out has room for it.
func (d *Device) ioctl(cmd uint32, set bool, data []byte, out []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ioctlLocked(cmd, set, data, out)
}

func (d *Device) ioctlLocked(cmd uint32, set bool, data []byte, out []byte) (int, error) {
	n := len(data)
	if len(out) > n {
		n = len(out)
	}
	var msg [bcdcHeaderLen + 1500]byte
	if n > len(msg)-bcdcHeaderLen {
		return 0, ErrFrameTooBig
	}
	d.reqID++
	put32(msg[0:], cmd)
	put32(msg[4:], uint32(n))
	flags := uint32(d.reqID) << bcdcIDShift
	if set {
		flags |= bcdcSet
	}
	put32(msg[8:], flags)
	put32(msg[12:], 0)
	copy(msg[bcdcHeaderLen:], data)

	d.ctlID = d.reqID
	d.ctlOK = false
	d.ctlBuf = nil
	if out != nil {
		d.ctlBuf = out[:0]
	}
	if err := d.send(chanControl, msg[:bcdcHeaderLen+n]); err != nil {
		return 0, err
	}
	for i := 0; i < 1000; i++ {
		d.receive()
		if d.ctlOK {
			d.ctlBuf, out = nil, d.ctlBuf
			if d.ctlErr {
				return 0, ErrIoctl
			}
			return len(out), nil
		}
		time.Sleep(time.Millisecond)
	}
	return 0, ErrTimeout
}

func (d *Device) setU32(cmd, v uint32) error {
	var b [4]byte
	put32(b[:], v)
	_, err := d.ioctl(cmd, true, b[:], nil)
	return err
}

// getVar reads a firmware variable ("iovar") into out.
func (d *Device) getVar(name string, out []byte) (int, error) {
	var req [64]byte
	n := copy(req[:], name)
	return d.ioctl(cmdGetVar, false, req[:n+1], out)
}

// setVar sets a firmware variable to value.
func (d *Device) setVar(name string, value []byte) error {
	var req [1500]byte
	n := copy(req[:], name)
	n++
	n += copy(req[n:], value)
	_, err := d.ioctl(cmdSetVar, true, req[:n], nil)
	return err
}

func (d *Device) setVarU32(name string, v uint32) error {
	var b [4]byte
	put32(b[:], v)
	return d.setVar(name, b[:])
}

// setBssVar sets a per interface variable: "bsscfg:" name, the interface, then the value.
func (d *Device) setBssVarU32(name string, v uint32) error {
	var b [8]byte
	put32(b[0:], 0)
	put32(b[4:], v)
	return d.setVar("bsscfg:"+name, b[:])
}

// loadCLM sends the CLM blob in 1400 byte pieces to "clmload" (brcmf_c_process_clm_blob).
func (d *Device) loadCLM(clm []byte) error {
	const chunk = 1400
	var req [12 + chunk]byte
	flag := uint16(1 << 12) // DLOAD_HANDLER_VER
	flag |= 0x2             // DL_BEGIN
	for len(clm) > 0 {
		n := len(clm)
		if n > chunk {
			n = chunk
		} else {
			flag |= 0x4 // DL_END
		}
		put16(req[0:], flag)
		put16(req[2:], 2) // DL_TYPE_CLM
		put32(req[4:], uint32(n))
		put32(req[8:], 0)
		copy(req[12:], clm[:n])
		if err := d.setVar("clmload", req[:12+n]); err != nil {
			return err
		}
		flag &^= 0x2
		clm = clm[n:]
	}
	var st [4]byte
	if _, err := d.getVar("clmload_status", st[:]); err == nil && le32(st[:]) != 0 {
		return ErrIoctl
	}
	return nil
}

// setup is the station setup of embassy-rs cyw43's init, after the firmware is up.
func (d *Device) setup() error {
	d.setVarU32("bus:txglom", 0)
	d.setVarU32("bus:rxglom", 0)
	d.setVarU32("apsta", 1)

	var mac [6]byte
	if _, err := d.getVar("cur_etheraddr", mac[:]); err != nil {
		return err
	}
	d.mac = mac

	var country [12]byte
	copy(country[0:], "XX")
	put32(country[4:], 0xffffffff)
	copy(country[8:], "XX")
	d.setVar("country", country[:])

	d.setU32(cmdSetAntdiv, 0)
	// Every event on, as the join waits on several.
	var mask [4 + 24]byte
	for i := 4; i < len(mask); i++ {
		mask[i] = 0xff
	}
	d.setVar("bsscfg:event_msgs", mask[:])

	if err := d.setU32(cmdUp, 0); err != nil {
		return err
	}
	d.setU32(cmdSetGMode, 1)
	d.setU32(cmdSetBand, 0)
	d.setVarU32("ampdu_ba_wsize", 8)
	d.setVarU32("ampdu_mpdu", 4)
	d.setU32(cmdSetPM, 0)
	return nil
}

// MAC returns the station's MAC address. It's the link MAC for tinygo/net/ip.
func (d *Device) MAC() [6]byte {
	return d.mac
}

// Send sends one Ethernet frame. It's the Link method for tinygo/net/ip.
func (d *Device) Send(frame []byte) error {
	if !d.joined {
		return ErrNotJoined
	}
	var b [bdcHeaderLen + maxFrame]byte
	if len(frame) > maxFrame {
		return ErrFrameTooBig
	}
	b[0] = bdcVersion2
	b[1] = 0
	b[2] = 0
	b[3] = 0
	n := copy(b[bdcHeaderLen:], frame)
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.send(chanData, b[:bdcHeaderLen+n])
}

// HandleFrames calls handler with each received Ethernet frame, for example the Input method of
// a tinygo/net/ip Stack. It polls the chip in its own goroutine. The frame is valid during the call.
func (d *Device) HandleFrames(handler func(frame []byte)) {
	d.handler = handler
	go d.poll()
}

func (d *Device) poll() {
	var frame [maxFrame]byte
	for {
		d.mu.Lock()
		got := d.receive()
		d.mu.Unlock()
		for {
			d.mu.Lock()
			n, ok := d.rxq.get(frame[:])
			d.mu.Unlock()
			if !ok {
				break
			}
			if d.handler != nil {
				d.handler(frame[:n])
			}
		}
		if !got {
			time.Sleep(time.Millisecond)
		}
	}
}

// FramesLost returns how many received frames were dropped for want of a buffer.
func (d *Device) FramesLost() uint32 {
	return d.rxq.lost
}
