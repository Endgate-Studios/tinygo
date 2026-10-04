// Package central is the BLE central role: it scans for advertisements, connects, and runs the
// GATT client. This file holds the setup and scanning, and conn.go the connection.
package central

import (
	"io"

	"runtime/memory"
	"tinygo/net/bluetooth/aclq"
	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

// Memory is where the reassembly and PDU buffers come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Transport sends one H4 packet to the controller.
type Transport interface {
	WriteHCI(packet []byte) error
}

// AdvHandler receives each advertising report. r.Data is valid only during the call.
type AdvHandler interface {
	Advertisement(r hci.AdvReport)
}

// ScanConfig is HCI_LE_Set_Scan_Parameters and the duplicate filter of HCI_LE_Set_Scan_Enable.
// Interval and Window are in units of 0.625 ms (Core spec v5.4 Vol 4 Part E 7.8.10).
type ScanConfig struct {
	Active           bool // send scan requests, so scan responses are reported too
	Interval, Window uint16
	OwnAddrType      uint8
	FilterDuplicates bool
}

// Config is the fixed setup of the central.
type Config struct {
	Scan       ScanConfig
	MaxMTU     uint16
	MaxACL     int    // LE ACL data packet length from HCI_LE_Read_Buffer_Size
	ACLBuffers uint16 // LE ACL buffer count from HCI_LE_Read_Buffer_Size
	QueueBytes int    // most bytes of ACL packets that wait for a buffer. 0 turns the queue off
	Notify     att.NotificationHandler
	Done       ProcedureHandler
	Conn       ConnectionHandler // optional
	Params     ParamPolicy       // optional. Without it, parameter update requests are rejected

	// Pairing is optional. It needs RNG and Bonds, and without them SMP frames get no answer.
	Own            smp.Address // the address that the controller connects with
	RNG            io.Reader
	Bonds          Bonds
	Bonding        bool
	IOCap          uint8
	Confirmer      smp.Confirmer
	PasskeyDisplay smp.PasskeyDisplay
	PasskeyInput   smp.PasskeyInput
	IRK            *[16]byte
	Identity       smp.Address // distributed with IRK when IRK is not nil
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilDependency Error = "central: memory, transport and handler cannot be nil"
	ErrConfig        Error = "central: scan window must be 4 to 16384 units and not above the interval, and MaxACL at least 27"
	ErrBusy          Error = "central: a scan command is in progress"
	ErrController    Error = "central: controller rejected a command, see Status"
	ErrNoCredits     Error = "central: controller has no free command buffer"
)

// State is the scanning state.
type State uint8

const (
	StateIdle State = iota
	StateStarting
	StateScanning
	StateStopping
	StateFailed
)

// Central is the central role of one controller, with one connection at a time.
type Central struct {
	mem     Memory
	tr      Transport
	cfg     Config
	scan    ScanConfig
	adv     AdvHandler
	flow    hci.Flow
	queue   aclq.Queue
	params  ConnParams
	link    link
	reasm   *l2cap.Reassembler
	client  *att.Client
	pairing *smp.Initiator
	peerID  smp.Address // address of the peer on this connection
	// found, foundPeer and ltk are scratch for bond lookups, zero between calls.
	found      smp.Bond
	foundPeer  smp.Address
	ltk        [16]byte
	encrypted  bool
	encStarted bool
	saved      bool
	peer       hci.Addr
	cmd        [32]byte
	rsp        [8]byte // ATT and signaling answers of the central
	handle     uint16
	mtu        uint16
	now        uint32
	frameErr   error
	state      State
	conn       ConnState
	status     uint8
}

// New returns a central. The scan setup is checked when Scan runs.
func New(mem Memory, tr Transport, cfg Config) (*Central, error) {
	if mem == nil || tr == nil {
		return nil, ErrNilDependency
	}
	if cfg.MaxACL < 27 {
		return nil, ErrConfig
	}
	if cfg.Notify == nil {
		cfg.Notify = ignore{}
	}
	if cfg.Done == nil {
		cfg.Done = ignore{}
	}
	if cfg.Conn == nil {
		cfg.Conn = ignore{}
	}
	c := &Central{mem: mem, tr: tr, cfg: cfg, scan: cfg.Scan, flow: hci.NewFlow(cfg.ACLBuffers), mtu: att.DefaultMTU}
	c.link.c = c
	c.queue.Init(mem, cfg.QueueBytes)
	var err error
	if c.reasm, err = l2cap.NewReassembler(mem, &c.link, max(int(cfg.MaxMTU), 65)); err != nil {
		return nil, err
	}
	if c.client, err = att.NewClient(mem, &c.link, cfg.Notify, cfg.MaxMTU); err != nil {
		return nil, err
	}
	if cfg.RNG != nil && cfg.Bonds != nil {
		if c.pairing, err = smp.NewInitiator(mem, cfg.RNG, &c.link, cfg.Confirmer, cfg.IOCap); err != nil {
			return nil, err
		}
		c.pairing.SetPasskeyIO(cfg.PasskeyDisplay, cfg.PasskeyInput)
		if cfg.IRK != nil {
			c.pairing.SetIdentity(cfg.IRK, cfg.Identity)
		}
	}
	return c, nil
}

// State returns the scanning state, and Status the controller status of ErrController.
func (c *Central) State() State  { return c.state }
func (c *Central) Status() uint8 { return c.status }

// Scan starts scanning and gives each report to h.
func (c *Central) Scan(h AdvHandler) error {
	if h == nil {
		return ErrNilDependency
	}
	if sc := c.scan; sc.Window < 4 || sc.Window > 0x4000 || sc.Interval < sc.Window {
		return ErrConfig
	}
	if c.state == StateStarting || c.state == StateStopping || c.conn == ConnConnecting {
		return ErrBusy
	}
	c.adv = h
	n, err := hci.PutLESetScanParams(c.cmd[:], c.scan.Active, c.scan.Interval, c.scan.Window, c.scan.OwnAddrType, 0)
	if err != nil {
		return err
	}
	if err := c.command(n); err != nil {
		return err
	}
	c.state = StateStarting
	return nil
}

// StopScan stops scanning. Reports that arrive before the controller confirms are still given out.
func (c *Central) StopScan() error {
	if c.state != StateScanning {
		return nil
	}
	n, err := hci.PutLESetScanEnable(c.cmd[:], false, false)
	if err != nil {
		return err
	}
	if err := c.command(n); err != nil {
		return err
	}
	c.state = StateStopping
	return nil
}

// HandleHCI takes one H4 packet from the controller.
func (c *Central) HandleHCI(packet []byte, now uint32) error {
	c.now = now
	if len(packet) == 0 {
		return nil
	}
	if packet[0] == hci.TypeACL {
		return c.acl(packet)
	}
	if packet[0] != hci.TypeEvent {
		return nil
	}
	e, err := hci.ParseEvent(packet)
	if err != nil {
		return err
	}
	c.flow.Update(e)
	if err := c.queue.Drain(&c.flow, c.tr); err != nil {
		return err
	}
	if cc, ok := e.CommandComplete(); ok {
		return c.commandComplete(cc)
	}
	if handled, err := c.connEvent(e); handled {
		return err
	}
	sub, params, ok := e.LEMeta()
	if !ok || sub != hci.SubAdvertisingReport || c.adv == nil || c.state == StateIdle {
		return nil
	}
	reports, err := hci.ParseAdvReports(params)
	if err != nil {
		return err
	}
	for r, ok := reports.Next(); ok; r, ok = reports.Next() {
		c.adv.Advertisement(r)
	}
	return nil
}

func (c *Central) commandComplete(cc hci.CommandComplete) error {
	var want hci.Opcode
	switch c.state {
	case StateStarting:
		want = hci.OpLESetScanParams
		if c.cmdSent() == hci.OpLESetScanEnable {
			want = hci.OpLESetScanEnable
		}
	case StateStopping:
		want = hci.OpLESetScanEnable
	default:
		return nil
	}
	if cc.Op != want {
		return nil
	}
	if len(cc.Return) == 0 || cc.Return[0] != 0 {
		c.state = StateFailed
		if len(cc.Return) > 0 {
			c.status = cc.Return[0]
		}
		return ErrController
	}
	switch {
	case c.state == StateStopping:
		c.state, c.adv = StateIdle, nil
	case want == hci.OpLESetScanParams:
		n, err := hci.PutLESetScanEnable(c.cmd[:], true, c.scan.FilterDuplicates)
		if err != nil {
			return err
		}
		return c.command(n)
	default:
		c.state = StateScanning
	}
	return nil
}

// cmdSent returns the opcode of the last command in c.cmd.
func (c *Central) cmdSent() hci.Opcode { return hci.Opcode(uint16(c.cmd[1]) | uint16(c.cmd[2])<<8) }

// command sends the first n bytes of c.cmd.
func (c *Central) command(n int) error {
	if !c.flow.CanSendCommand() {
		return ErrNoCredits
	}
	c.flow.TookCommand()
	return c.tr.WriteHCI(c.cmd[:n])
}

// ADField returns the first field of type typ in advertising data, for example 0x09 for the
// complete local name (Core spec Supplement v11 Part A 1). ok is false when there is none.
func ADField(data []byte, typ uint8) (field []byte, ok bool) {
	for len(data) > 0 {
		n := int(data[0])
		if n == 0 {
			return nil, false // the rest is padding
		}
		if 1+n > len(data) {
			return nil, false
		}
		if data[1] == typ {
			return data[2 : 1+n], true
		}
		data = data[1+n:]
	}
	return nil, false
}
