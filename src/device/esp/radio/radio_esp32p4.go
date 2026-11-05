//go:build esp32p4

package radio

// The ESP32-P4 has no radio. On a board with an ESP32-C6, such as Elecrow's CrowPanel Advanced, the radio is the C6's, running
// Espressif's ESP-Hosted-MCU 2.x firmware, on the SD/MMC host's slot 1 as an SDIO device, 4 data lines wide. This is the same API as
// on the chips with a radio: the C6 does the Wi-Fi, told what to do by control messages, and passes Ethernet frames, which this
// computer's own network stack takes, as it does from the S3's radio.
//
// It follows ESP-IDF's host side at v2.12.12: host/drivers/transport/sdio/sdio_drv.c and
// host/port/esp/freertos/src/port_esp_hosted_host_sdio.c. It polls the C6 rather than taking its interrupt. The wire format is in
// hosted_wire.go.

import (
	"encoding/binary"
	"machine"
	"sync"
	"time"
)

const (
	ErrNoRadio    HostedError = "radio: no ESP32-C6 answered on SDIO"
	ErrNotStarted HostedError = "radio: Init hasn't been called, or failed"
	ErrNoAnswer   HostedError = "radio: the ESP32-C6 didn't answer"
	ErrRefused    HostedError = "radio: the ESP32-C6 said no"
	ErrTooLong    HostedError = "radio: a frame that's too long"
	ErrBusy       HostedError = "radio: the ESP32-C6 has no room"
	ErrTooFew     HostedError = "radio: the ESP32-C6's SDIO has no function 1"
)

// FrameSize is the size of a received frame's buffer.
const FrameSize = 1600

// Mode is the Wi-Fi mode.
type Mode int32

const (
	ModeNull Mode = iota
	ModeStation
	ModeAccessPoint
	ModeStationAndAccessPoint
)

// PowerSave is the modem power save of the station.
type PowerSave int32

const (
	PowerSaveNone PowerSave = iota
	PowerSaveMinModem
	PowerSaveMaxModem
)

// AuthMode is how a network is secured.
type AuthMode int32

const AuthOpen AuthMode = 0

// AP is a network that a scan heard.
type AP struct {
	SSID    string
	BSSID   [6]byte
	Channel uint8
	RSSI    int8
	Auth    AuthMode
}

// EventID is a Wi-Fi event, numbered as ESP-IDF's wifi_event_t.
type EventID int32

const (
	EventStaConnected    EventID = 4
	EventStaDisconnected EventID = 5
)

// Event is a Wi-Fi event.
type Event struct {
	ID   EventID
	Data []byte

	bssid   [6]byte
	channel uint8
	reason  uint8
}

// DisconnectReason gives why the station disconnected: ESP-IDF's wifi_err_reason_t.
func (e Event) DisconnectReason() uint8 { return e.reason }

// AccessPoint gives the BSSID and channel of an EventStaConnected.
func (e Event) AccessPoint() (bssid [6]byte, channel uint8) { return e.bssid, e.channel }

// Frame is a received Ethernet frame.
type Frame struct {
	buf *[FrameSize]byte
	n   int
}

// Data gives the frame from the Ethernet header.
func (f Frame) Data() []byte { return f.buf[:f.n] }

// Release gives the buffer back.
func (f Frame) Release() {
	select {
	case freeFrames <- f.buf:
	default:
	}
}

// Option is an option of Init.
type Option func()

// WithArenaMemory is the S3's: the C6 has its own memory.
func WithArenaMemory(mem []byte) Option { return func() {} }

// WithPSRAMArena is the S3's: the C6 has its own memory.
func WithPSRAMArena(mem []byte) Option { return func() {} }

// Station is the station interface, for a network stack.
type Station struct{}

// MAC gives the MAC address of the station.
func (Station) MAC() [6]byte { return MAC() }

// Send sends an Ethernet frame.
func (Station) Send(frame []byte) error { return Send(frame) }

const (
	// The C6's SDIO registers, function 1's (host/drivers/transport/sdio/sdio_reg.h). It only decodes their low 10 bits.
	regTokenRData = 0x044
	regIntRaw     = 0x050 // and on from here, to PACKET_LEN at 0x060: read in one go
	regIntClear   = 0x0D4
	regToSlave    = 0x08C // SLCHOST_CONF_W7: each bit a host interrupt

	endAddress  = 0x1F800 // ESP_SLAVE_CMD53_END_ADDR: data's address is this less its length
	lenMask     = 0xFFFFF
	rxByteMax   = 0x100000
	txBufferMax = 0x1000
	blockSize   = 512

	intNewPacket     = 1 << 23
	intStartThrottle = 1 << 7
	intStopThrottle  = 1 << 6

	toSlaveOpenDataPath = 1 << 0

	// CCCR and FBR (SD's SDIO spec).
	cccrFnEnable   = 0x02
	cccrFnReady    = 0x03
	cccrIntEnable  = 0x04
	cccrIOReset    = 0x06
	cccrBusWidth   = 0x07
	cccrCapability = 0x08
	cccrBlockSize  = 0x10
	fbr1           = 0x100

	cmdGoIdle       = 0
	cmdSendRCA      = 3
	cmdIOSendOpCond = 5
	cmdSelect       = 7
	cmdIODirect     = 52
	cmdIOExtended   = 53

	rpcTimeout  = 5 * time.Second
	scanTimeout = 15 * time.Second
	framePool   = 16
)

// DMA memory: package variables, so internal RAM, which the SD/MMC host's DMA reaches.
var (
	rxArray   [16<<10 + 4]byte
	txArray   [2<<10 + 4]byte
	regsArray [64 + 4]byte
)

var link struct {
	mu       sync.Mutex // the bus, and what counts what's gone over it
	slot     *machine.SDMMCSlot
	rx, tx   []byte // DMA memory
	regs     []byte
	stream   []byte // what's been read and not yet taken: a frame can be cut by the end of a read
	txCount  uint32 // buffers the C6 has had, mod txBufferMax
	rxCount  uint32 // bytes read from it, mod rxByteMax
	txFree   uint32
	seq      uint16
	checksum bool
	throttle bool
	started  bool
	version  uint32 // the C6's ESP-Hosted: major << 16 | minor << 8 | patch

	initDone chan hostedPrivInit
	serial   []byte // a control message's fragments, until the last

	rpcMu   sync.Mutex // one control message at a time
	uid     uint32
	wantID  uint32
	waiting chan rpcMessage

	mac [6]byte
}

var (
	events     = make(chan Event, 8)
	frames     = make(chan Frame, framePool)
	freeFrames = make(chan *[FrameSize]byte, framePool)
	dropped    uint32
)

// Init brings up the C6: resets it, starts its SDIO, and waits for it to say what it is, then has it start its Wi-Fi. It takes about
// two seconds.
func Init(opts ...Option) error {
	if link.started {
		return nil
	}
	var err error
	if link.rx, err = machine.SDMMCMemory(rxArray[:]); err != nil {
		return err
	}
	if link.tx, err = machine.SDMMCMemory(txArray[:]); err != nil {
		return err
	}
	if link.regs, err = machine.SDMMCMemory(regsArray[:]); err != nil {
		return err
	}
	link.stream = make([]byte, 0, len(link.rx)+hostedMaxFrame)
	for range framePool {
		freeFrames <- new([FrameSize]byte)
	}
	link.initDone = make(chan hostedPrivInit, 1)
	link.waiting = make(chan rpcMessage, 1)

	// Reset the C6 by its EN pin, as transport_gpio_reset does, and give it the time ESP-Hosted's default does to start.
	en := machine.C6_EN_PIN
	en.Configure(machine.PinConfig{Mode: machine.PinOutput})
	en.High()
	time.Sleep(10 * time.Millisecond)
	en.Low()
	time.Sleep(10 * time.Millisecond)
	en.High()
	time.Sleep(1500 * time.Millisecond)

	slot, err := machine.OpenSDMMC(machine.SDMMCConfig{
		Slot: 1, Width: 4,
		CLK: machine.C6_SDIO_CLK_PIN, CMD: machine.C6_SDIO_CMD_PIN,
		D0: machine.C6_SDIO_D0_PIN, D1: machine.C6_SDIO_D1_PIN, D2: machine.C6_SDIO_D2_PIN, D3: machine.C6_SDIO_D3_PIN,
	})
	if err != nil {
		return err
	}
	link.slot = slot
	for tries := 0; ; tries++ {
		if err = startCard(); err == nil {
			break
		}
		if tries == 15 { // 1.5 s, as ESP-Hosted's CARD_INIT_TIMEOUT_MS
			slot.Close()
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := writeByte(1, regToSlave, toSlaveOpenDataPath); err != nil {
		return err
	}
	go receive()

	var hello hostedPrivInit
	select {
	case hello = <-link.initDone:
	case <-time.After(3 * time.Second):
		return ErrNoAnswer
	}
	link.checksum = hello.capability&hostedCapChecksum != 0
	if err := send(hostedIfPriv, hostedPacketTypeEvent, hostConfig(hello.chipID)); err != nil {
		return err
	}
	link.started = true

	if _, err := call(rpcWifiInit, func(w *pbWriter) { w.message(1, wifiInitConfig()) }, rpcTimeout); err != nil {
		link.started = false
		return err
	}
	if answer, err := call(rpcGetMACAddress, func(*pbWriter) {}, rpcTimeout); err == nil {
		pbFields(answer, func(f pbField) {
			if f.num == 1 {
				copy(link.mac[:], f.b)
			}
		})
	}
	return nil
}

// startCard is sdmmc_card_init for an SDIO device, and hosted_sdio_card_fn_init: function 1 on, its interrupt, 512-byte blocks, 4
// data lines, and 20 MHz.
func startCard() error {
	s := link.slot
	if err := s.SetWidth(1); err != nil {
		return err
	}
	if err := s.SetClock(400); err != nil {
		return err
	}
	writeByte(0, cccrIOReset, 0x08) // RES, which it may not answer
	s.Command(cmdGoIdle, 0, machine.SDMMCInit)
	ocr, err := s.Command(cmdIOSendOpCond, 0, machine.SDMMCResponseNoCRC)
	if err != nil {
		return err
	}
	if ocr>>28&7 < 1 {
		return ErrTooFew
	}
	for tries := 0; ; tries++ {
		r, err := s.Command(cmdIOSendOpCond, ocr&0xFF8000, machine.SDMMCResponseNoCRC)
		if err != nil {
			return err
		}
		if r&(1<<31) != 0 {
			break
		}
		if tries == 100 {
			return ErrNoRadio
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, err := s.Command(cmdSendRCA, 0, machine.SDMMCResponse)
	if err != nil {
		return err
	}
	if _, err := s.Command(cmdSelect, r&0xFFFF0000, machine.SDMMCResponse); err != nil {
		return err
	}

	// Function 1, and wait for it to be ready.
	ioe, err := readByte(0, cccrFnEnable)
	if err != nil {
		return err
	}
	if err := writeByte(0, cccrFnEnable, ioe|0x02); err != nil {
		return err
	}
	for tries := 0; ; tries++ {
		ior, err := readByte(0, cccrFnReady)
		if err != nil {
			return err
		}
		if ior&0x02 != 0 {
			break
		}
		if tries == 100 {
			return ErrNoRadio
		}
		time.Sleep(10 * time.Millisecond)
	}
	ie, err := readByte(0, cccrIntEnable)
	if err != nil {
		return err
	}
	if err := writeByte(0, cccrIntEnable, ie|0x03); err != nil { // the master enable, and function 1's
		return err
	}
	for _, fn := range []uint32{0, fbr1} {
		if err := writeByte(0, fn+cccrBlockSize, blockSize&0xFF); err != nil {
			return err
		}
		if err := writeByte(0, fn+cccrBlockSize+1, blockSize>>8); err != nil {
			return err
		}
	}

	// Four data lines, unless it's a low-speed card that can't.
	capability, err := readByte(0, cccrCapability)
	if err != nil {
		return err
	}
	if capability&0x40 == 0 || capability&0x80 != 0 {
		width, err := readByte(0, cccrBusWidth)
		if err != nil {
			return err
		}
		if err := writeByte(0, cccrBusWidth, width&^3|2); err != nil {
			return err
		}
		if err := s.SetWidth(4); err != nil {
			return err
		}
	}
	return s.SetClock(20_000)
}

// readByte is CMD52, a read of one register.
func readByte(fn, reg uint32) (uint8, error) {
	r, err := link.slot.Command(cmdIODirect, fn<<28|(reg&0x1FFFF)<<9, machine.SDMMCResponse)
	return uint8(r), err
}

// writeByte is CMD52, a write of one register.
func writeByte(fn, reg uint32, v uint8) error {
	_, err := link.slot.Command(cmdIODirect, 1<<31|fn<<28|(reg&0x1FFFF)<<9|uint32(v), machine.SDMMCResponse)
	return err
}

// transfer is CMD53 on function 1, from or to addr, which goes up as it goes. buf is DMA memory, a multiple of 4: whole blocks, and
// then what's left in byte mode, as sdio_read_fromio and sdio_write_toio do.
func transfer(addr uint32, buf []byte, write bool) error {
	arg := uint32(1<<28 | 1<<26) // function 1, incrementing
	if write {
		arg |= 1 << 31
	}
	for len(buf) >= blockSize {
		n := min(len(buf)&^(blockSize-1), machine.SDMMCMaxTransfer)
		if err := link.slot.Transfer(cmdIOExtended, arg|1<<27|(addr&0x1FFFF)<<9|uint32(n/blockSize), buf[:n], blockSize, write, false); err != nil {
			return err
		}
		buf, addr = buf[n:], addr+uint32(n)
	}
	if len(buf) > 0 {
		return link.slot.Transfer(cmdIOExtended, arg|(addr&0x1FFFF)<<9|uint32(len(buf)), buf, len(buf), write, false)
	}
	return nil
}

// receive reads what the C6 has, for as long as the board runs.
func receive() {
	idle := 0
	for {
		got, _ := receiveOnce()
		if got {
			idle = 0
			continue
		}
		// Nothing: a short wait at first, and longer once it's been quiet.
		idle++
		if idle < 50 {
			time.Sleep(200 * time.Microsecond)
		} else {
			time.Sleep(time.Millisecond)
		}
	}
}

// receiveOnce reads the C6's registers, and when it has a packet, as many bytes as it says it has, which can be several frames back to
// back: sdio_read_task.
func receiveOnce() (bool, error) {
	link.mu.Lock()
	defer link.mu.Unlock()
	regs := link.regs[:20]
	if err := transfer(regIntRaw, regs, false); err != nil {
		return false, err
	}
	interrupts := binary.LittleEndian.Uint32(regs)
	pending := binary.LittleEndian.Uint32(regs[16:])
	if interrupts != 0 {
		ack := link.regs[32:36]
		binary.LittleEndian.PutUint32(ack, interrupts)
		transfer(regIntClear, ack, true)
	}
	if interrupts&intStartThrottle != 0 {
		link.throttle = true
	}
	if interrupts&intStopThrottle != 0 {
		link.throttle = false
	}
	if interrupts&intNewPacket == 0 || pending == 0xFFFFFFFF {
		return false, nil
	}
	n := (pending&lenMask + rxByteMax - link.rxCount) % rxByteMax
	if n == 0 {
		return false, nil
	}
	// Read it all, a buffer at a time: what's left at the address that's that much short of the end.
	left := n
	var err error
	for left > 0 {
		take := min(left, uint32(len(link.rx)))
		buf := link.rx[:(take+3)&^3]
		if err = transfer(endAddress-left, buf, false); err != nil {
			break
		}
		left -= take
		link.stream = append(link.stream, buf[:take]...)
		used := deliverFrames(link.stream, left == 0)
		link.stream = link.stream[:copy(link.stream, link.stream[used:])]
	}
	link.stream = link.stream[:0]
	link.rxCount = (link.rxCount + n) % rxByteMax
	return true, err
}

// deliverFrames takes the frames in b, and gives how much of b they were: all of it, or up to a frame that's cut short, unless it's
// the last of a read. link.mu is held.
func deliverFrames(b []byte, last bool) int {
	used := 0
	for len(b)-used >= hostedHeaderLen {
		h, ok := parseHostedHeader(b[used:])
		if !ok {
			return len(b) // padding, or what can't be read, and so nor can the rest
		}
		size := hostedHeaderLen + int(h.length)
		if len(b)-used < size {
			if last {
				return len(b)
			}
			return used
		}
		frame := b[used : used+size]
		if !link.checksum || hostedChecksum(frame) == h.checksum {
			deliver(h, frame[hostedHeaderLen:])
		}
		used += size
	}
	if last {
		return len(b)
	}
	return used
}

// deliver takes one frame. link.mu is held.
func deliver(h hostedHeader, data []byte) {
	switch h.ifType {
	case hostedIfSTA:
		var buf *[FrameSize]byte
		select {
		case buf = <-freeFrames:
		default:
			dropped++
			return
		}
		n := copy(buf[:], data)
		select {
		case frames <- Frame{buf: buf, n: n}:
		default:
			freeFrames <- buf
			dropped++
		}
	case hostedIfSerial:
		link.serial = append(link.serial, data...)
		if h.flags&hostedMoreFragment != 0 {
			return
		}
		endpoint, proto, ok := parseSerialTLV(link.serial)
		link.serial = link.serial[:0]
		if !ok {
			return
		}
		m, ok := parseRPC(proto)
		if !ok {
			return
		}
		if endpoint == hostedEndpointEvent || m.kind == rpcEvent {
			event(m)
			return
		}
		if m.id == link.wantID {
			m.payload = append([]byte(nil), m.payload...) // what it's in is read into again
			select {
			case link.waiting <- m:
			default:
			}
		}
	case hostedIfPriv:
		if h.privType != hostedPacketTypeEvent {
			return
		}
		if p, ok := parsePrivInit(data); ok {
			link.version = p.version
			select {
			case link.initDone <- p:
			default:
			}
		}
	}
}

// event takes what the C6 says has happened.
func event(m rpcMessage) {
	var e Event
	switch m.id {
	case rpcEventStaConnected:
		e.ID = EventStaConnected
		pbFields(m.payload, func(f pbField) {
			if f.num == 2 {
				pbFields(f.b, func(g pbField) {
					switch g.num {
					case 3:
						copy(e.bssid[:], g.b)
					case 4:
						e.channel = uint8(g.v)
					}
				})
			}
		})
	case rpcEventStaDisconnected:
		e.ID = EventStaDisconnected
		pbFields(m.payload, func(f pbField) {
			if f.num == 2 {
				pbFields(f.b, func(g pbField) {
					if g.num == 4 {
						e.reason = uint8(g.v)
					}
				})
			}
		})
	default:
		return
	}
	select {
	case events <- e:
	default:
	}
}

// send sends a frame: data after a header, in fragments if a control message is too long for one, once the C6 has room.
func send(ifType, privType uint8, data []byte) error {
	link.mu.Lock()
	defer link.mu.Unlock()
	for {
		n := min(len(data), hostedMaxData)
		h := hostedHeader{ifType: ifType, length: uint16(n), privType: privType, seq: link.seq}
		if n < len(data) {
			if ifType != hostedIfSerial {
				return ErrTooLong
			}
			h.flags = hostedMoreFragment
		}
		link.seq++
		total := hostedHeaderLen + n
		frame := link.tx[:total]
		copy(frame[hostedHeaderLen:], data[:n])
		putHostedHeader(frame, h)
		if link.checksum {
			binary.LittleEndian.PutUint16(frame[6:], hostedChecksum(frame))
		}
		if err := waitForRoom(); err != nil {
			return err
		}
		if err := transfer(endAddress-uint32(total), link.tx[:(total+3)&^3], true); err != nil {
			return err
		}
		link.txCount = (link.txCount + 1) % txBufferMax
		link.txFree--
		data = data[n:]
		if len(data) == 0 {
			return nil
		}
	}
}

// waitForRoom waits until the C6 has a buffer for what's going: sdio_is_write_buffer_available. link.mu is held.
func waitForRoom() error {
	wait := 400 * time.Microsecond
	for tries := 0; link.txFree == 0; tries++ {
		regs := link.regs[:4]
		if err := transfer(regTokenRData, regs, false); err != nil {
			return err
		}
		token := binary.LittleEndian.Uint32(regs)
		link.txFree = (token>>16&0xFFF + txBufferMax - link.txCount) % txBufferMax
		if link.txFree > 0 {
			break
		}
		if tries == 50 {
			return ErrBusy
		}
		link.mu.Unlock()
		time.Sleep(wait)
		link.mu.Lock()
		wait = min(wait+400*time.Microsecond, 30*time.Millisecond)
	}
	return nil
}

// call sends a control message, and gives its answer once it's come, if it says it's done: an answer's resp, esp_err_t, is 0 for
// done.
func call(id uint32, fill func(*pbWriter), timeout time.Duration) ([]byte, error) {
	link.rpcMu.Lock()
	defer link.rpcMu.Unlock()
	var m pbWriter
	fill(&m)
	link.uid++
	link.mu.Lock()
	link.wantID = id + rpcRespOffset
	link.mu.Unlock()
	select {
	case <-link.waiting: // an answer that came too late for the last
	default:
	}
	if err := send(hostedIfSerial, 0, serialTLV(nil, rpcEnvelope(id, link.uid, m))); err != nil {
		return nil, err
	}
	select {
	case answer := <-link.waiting:
		field := 1
		if id == rpcGetMACAddress {
			field = 2
		}
		if rpcResult(answer.payload, field) != 0 {
			return answer.payload, ErrRefused
		}
		return answer.payload, nil
	case <-time.After(timeout):
		return nil, ErrNoAnswer
	}
}

// SetMode sets the Wi-Fi mode.
func SetMode(m Mode) error {
	if !link.started {
		return ErrNotStarted
	}
	_, err := call(rpcSetWifiMode, func(w *pbWriter) { w.int(1, int32(m)) }, rpcTimeout)
	return err
}

// Start starts the Wi-Fi.
func Start() error {
	if !link.started {
		return ErrNotStarted
	}
	_, err := call(rpcWifiStart, func(*pbWriter) {}, rpcTimeout)
	return err
}

// Scan is the networks heard on every channel, as the C6 lists them.
func Scan() ([]AP, error) {
	if !link.started {
		return nil, ErrNotStarted
	}
	if _, err := call(rpcWifiScanStart, func(w *pbWriter) { w.bool(2, true) }, scanTimeout); err != nil {
		return nil, err
	}
	answer, err := call(rpcWifiScanGetApNum, func(*pbWriter) {}, rpcTimeout)
	if err != nil {
		return nil, err
	}
	var n int32
	pbFields(answer, func(f pbField) {
		if f.num == 2 {
			n = int32(f.v)
		}
	})
	if n <= 0 {
		return nil, nil
	}
	n = min(n, 32)
	if answer, err = call(rpcWifiScanGetAps, func(w *pbWriter) { w.int(1, n) }, rpcTimeout); err != nil {
		return nil, err
	}
	var aps []AP
	pbFields(answer, func(f pbField) {
		if f.num == 3 {
			aps = append(aps, parseAPRecord(f.b))
		}
	})
	return aps, nil
}

// Connect joins a network by its name.
func Connect(ssid, password string) error {
	return connect(ssid, password, [6]byte{}, 0, false)
}

// ConnectTo joins a network at one of its access points.
func ConnectTo(ssid, password string, bssid [6]byte, channel uint8) error {
	return connect(ssid, password, bssid, channel, true)
}

func connect(ssid, password string, bssid [6]byte, channel uint8, pick bool) error {
	if !link.started {
		return ErrNotStarted
	}
	cfg := staConfig(ssid, password, bssid, channel, pick)
	if _, err := call(rpcWifiSetConfig, func(w *pbWriter) { w.message(2, cfg) }, rpcTimeout); err != nil {
		return err
	}
	_, err := call(rpcWifiConnect, func(*pbWriter) {}, rpcTimeout)
	return err
}

// Signal is the strength of the access point joined, in dBm.
func Signal() (int8, error) {
	if !link.started {
		return 0, ErrNotStarted
	}
	answer, err := call(rpcWifiStaGetApInfo, func(*pbWriter) {}, rpcTimeout)
	if err != nil {
		return 0, err
	}
	var ap AP
	pbFields(answer, func(f pbField) {
		if f.num == 2 {
			ap = parseAPRecord(f.b)
		}
	})
	return ap.RSSI, nil
}

// Disconnect leaves the network.
func Disconnect() error {
	if !link.started {
		return ErrNotStarted
	}
	_, err := call(rpcWifiDisconnect, func(*pbWriter) {}, rpcTimeout)
	return err
}

// SetPowerSave sets the modem's power save.
func SetPowerSave(ps PowerSave) error {
	if !link.started {
		return ErrNotStarted
	}
	_, err := call(rpcWifiSetPs, func(w *pbWriter) { w.int(1, int32(ps)) }, rpcTimeout)
	return err
}

// Events are what's happened.
func Events() <-chan Event { return events }

// Frames are the Ethernet frames that have come.
func Frames() <-chan Frame { return frames }

// FramesDropped is how many frames there was no room for.
func FramesDropped() uint32 { return dropped }

// MAC is the station's MAC address, the C6's.
func MAC() [6]byte { return link.mac }

// Send sends an Ethernet frame.
func Send(frame []byte) error {
	if !link.started {
		return ErrNotStarted
	}
	if len(frame) > hostedMaxData {
		return ErrTooLong
	}
	return send(hostedIfSTA, 0, frame)
}

// FirmwareVersion is the C6's ESP-Hosted, as major, minor and patch, once Init's heard it.
func FirmwareVersion() (major, minor, patch uint8) {
	return uint8(link.version >> 16), uint8(link.version >> 8), uint8(link.version)
}
