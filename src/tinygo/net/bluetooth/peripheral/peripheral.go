// Package peripheral is a BLE peripheral host for one connection at a time. It joins hci, l2cap,
// att and smp: it advertises, routes L2CAP channels, answers LTK requests and saves bonds.
package peripheral

import (
	"io"

	"runtime/memory"
	"tinygo/net/bluetooth/aclq"
	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

// Memory is where the reassembly, PDU and pairing buffers come from.
type Memory interface {
	memory.Allocator
	memory.Freer
}

// Transport sends one H4 packet, a command or ACL data, to the controller.
type Transport interface {
	WriteHCI(packet []byte) error
}

// Bonds stores bonds, for example in flash. Bond copies entry i and reports false after the last
// entry. peer is the address that the peer used when it paired.
type Bonds interface {
	Bond(i int, peer *smp.Address, b *smp.Bond) bool
	SaveBond(peer *smp.Address, b *smp.Bond) error
}

// CCCDStore keeps the CCCD values of bonded peers, in the order of att.Server.CopyCCCDs.
// peer is the identity address of the bond. LoadCCCDs reports false when it has no values.
type CCCDStore interface {
	LoadCCCDs(peer *smp.Address, dst []uint16) bool
	SaveCCCDs(peer *smp.Address, values []uint16) error
}

// ProcedureHandler receives the result of each GATT client procedure.
type ProcedureHandler interface {
	ProcedureDone(err error)
}

// Config is the fixed setup of the peripheral.
type Config struct {
	Own        smp.Address // the address that the controller advertises with
	Adv        hci.AdvParams
	AdvData    []byte // at most 31 bytes
	IOCap      uint8
	MaxMTU     uint16
	MaxACL     int    // LE ACL data packet length from HCI_LE_Read_Buffer_Size
	ACLBuffers uint16 // LE ACL buffer count from HCI_LE_Read_Buffer_Size
	QueueBytes int    // most bytes of ACL packets that wait for a buffer. 0 turns the queue off
	IRK        *[16]byte
	Identity   smp.Address // distributed with IRK when IRK is not nil
	Confirmer  smp.Confirmer
	// PasskeyDisplay and PasskeyInput are optional. Pairing that needs a missing one fails.
	PasskeyDisplay smp.PasskeyDisplay
	PasskeyInput   smp.PasskeyInput
	Notify         att.NotificationHandler
	Done           ProcedureHandler
	CCCDs          CCCDStore // optional. Without it, a bonded peer subscribes again after it reconnects
}

// Error is a string error type, so error values are constants in flash and not in RAM.
type Error string

func (e Error) Error() string { return string(e) }

const (
	ErrNilDependency Error = "peripheral: memory, transport, random source, database and bonds cannot be nil"
	ErrConfig        Error = "peripheral: advertising data is longer than 31 bytes or MaxACL is below 27"
	ErrController    Error = "peripheral: controller rejected a setup command, see Status"
	ErrNoCredits     Error = "peripheral: controller has no free command or ACL buffer"
	ErrNoMemory      Error = "peripheral: out of memory"
	ErrClosed        Error = "peripheral: closed"
)

// ErrQueueFull is returned when the ACL queue has no room, see Config.QueueBytes.
const ErrQueueFull = aclq.ErrFull

// State is the state of the peripheral.
type State uint8

const (
	StateIdle State = iota
	StateStarting
	StateAdvertising
	StateConnected
	StateFailed
)

// setup is the command sequence that starts advertising. After a disconnection only the last step runs.
var setup = [...]hci.Opcode{hci.OpReset, hci.OpLESetAdvParams, hci.OpLESetAdvData, hci.OpLESetAdvEnable}

// Peripheral is the host. Call HandleHCI with each packet from the controller and Tick often.
type Peripheral struct {
	mem       Memory
	tr        Transport
	bonds     Bonds
	cfg       Config
	link      link
	reasm     *l2cap.Reassembler
	server    *att.Server
	client    *att.Client
	pairing   *smp.Responder
	flow      hci.Flow
	queue     aclq.Queue
	peer      smp.Address
	bondKey   smp.Address // identity of the bond of this peer, valid when bonded
	bonded    bool
	foundPeer smp.Address
	found     smp.Bond // scratch for bond lookups, zero between calls
	ltk       [16]byte // scratch for the LTK reply, zero between calls
	cmd       [40]byte
	sig       [8]byte
	handle    uint16
	now       uint32
	frameErr  error
	state     State
	step      uint8
	status    uint8
	encrypted bool
	saved     bool
	closed    bool
}

// New returns a peripheral. db is the GATT database of the server, and the caller closes it.
func New(mem Memory, tr Transport, rng io.Reader, db *att.DB, bonds Bonds, cfg Config) (*Peripheral, error) {
	if mem == nil || tr == nil || rng == nil || db == nil || bonds == nil {
		return nil, ErrNilDependency
	}
	if len(cfg.AdvData) > 31 || cfg.MaxACL < 27 {
		return nil, ErrConfig
	}
	if cfg.Notify == nil {
		cfg.Notify = ignore{}
	}
	if cfg.Done == nil {
		cfg.Done = ignore{}
	}
	p := &Peripheral{mem: mem, tr: tr, bonds: bonds, cfg: cfg, flow: hci.NewFlow(cfg.ACLBuffers)}
	p.link.p = p
	p.queue.Init(mem, cfg.QueueBytes)
	var err error
	if p.reasm, err = l2cap.NewReassembler(mem, &p.link, max(int(cfg.MaxMTU), 65)); err != nil {
		return nil, err
	}
	if p.server, err = att.NewServer(mem, db, &p.link, cfg.MaxMTU); err != nil {
		return nil, err
	}
	if p.client, err = att.NewClient(mem, &p.link, cfg.Notify, cfg.MaxMTU); err != nil {
		p.server.Close()
		return nil, err
	}
	if p.pairing, err = smp.NewResponder(mem, rng, &p.link, cfg.Confirmer, cfg.IOCap); err != nil {
		p.server.Close()
		return nil, err
	}
	if cfg.IRK != nil {
		p.pairing.SetIdentity(cfg.IRK, cfg.Identity)
	}
	p.pairing.SetPasskeyIO(cfg.PasskeyDisplay, cfg.PasskeyInput)
	return p, nil
}

// Server returns the ATT server, for example to send notifications.
func (p *Peripheral) Server() *att.Server { return p.server }

// Client returns the GATT client. ProcedureHandler receives the result of each procedure.
func (p *Peripheral) Client() *att.Client { return p.client }

// State returns the state, Status the controller status of ErrController, and Encrypted whether
// the link is encrypted now.
func (p *Peripheral) State() State       { return p.state }
func (p *Peripheral) Status() uint8      { return p.status }
func (p *Peripheral) Encrypted() bool    { return p.encrypted }
func (p *Peripheral) Pairing() smp.State { return p.pairing.State() }

// Start resets the controller and starts advertising.
func (p *Peripheral) Start() error {
	if p.closed {
		return ErrClosed
	}
	p.state, p.step = StateStarting, 0
	return p.sendStep()
}

// Close frees all buffers and pairing state.
func (p *Peripheral) Close() {
	p.queue.Reset()
	p.reasm.Close()
	p.server.Close()
	p.client.Close()
	p.pairing.Close()
	p.closed = true
}

// Tick runs the SMP and ATT timeouts.
func (p *Peripheral) Tick(now uint32) error {
	if p.closed {
		return ErrClosed
	}
	p.pairing.Tick(now)
	if done, err := p.client.Tick(now); done {
		p.cfg.Done.ProcedureDone(err)
	}
	return nil
}

// HandleHCI takes one H4 packet from the controller.
func (p *Peripheral) HandleHCI(packet []byte, now uint32) error {
	if p.closed {
		return ErrClosed
	}
	p.now = now
	if len(packet) == 0 {
		return nil
	}
	switch packet[0] {
	case hci.TypeEvent:
		e, err := hci.ParseEvent(packet)
		if err != nil {
			return err
		}
		p.flow.Update(e)
		if err := p.drain(); err != nil {
			return err
		}
		return p.event(e)
	case hci.TypeACL:
		a, err := hci.ParseACL(packet)
		if err != nil || p.state != StateConnected || a.Handle != p.handle {
			return err
		}
		p.frameErr = nil
		if err := p.reasm.HandleACL(a); err != nil {
			return err
		}
		return p.frameErr
	}
	return nil
}

func (p *Peripheral) event(e hci.Event) error {
	if cc, ok := e.CommandComplete(); ok {
		return p.commandComplete(cc)
	}
	if d, ok := e.Disconnection(); ok {
		if p.state == StateConnected && d.Handle == p.handle {
			return p.disconnected()
		}
		return nil
	}
	if ec, ok := e.EncryptionChange(); ok {
		if p.state != StateConnected || ec.Handle != p.handle {
			return nil
		}
		p.encrypted = ec.Status == 0 && ec.Enabled
		p.server.SetSecurity(p.encrypted, p.bonded)
		if p.encrypted {
			if err := p.pairing.Encrypted(p.now); err != nil {
				return err
			}
			if err := p.saveBond(); err != nil {
				return err
			}
			return p.loadCCCDs()
		}
		return nil
	}
	sub, params, ok := e.LEMeta()
	if !ok {
		return nil
	}
	switch sub {
	case hci.SubConnectionComplete:
		c, err := hci.ParseConnection(params)
		if err != nil || c.Status != 0 || c.Role != 1 {
			return err
		}
		p.connected(c)
	case hci.SubLongTermKeyRequest:
		r, err := hci.ParseLTKRequest(params)
		if err != nil {
			return err
		}
		return p.ltkRequest(r)
	}
	return nil
}

func (p *Peripheral) commandComplete(cc hci.CommandComplete) error {
	if p.state != StateStarting || int(p.step) >= len(setup) || cc.Op != setup[p.step] {
		return nil
	}
	if len(cc.Return) == 0 || cc.Return[0] != 0 {
		p.state = StateFailed
		if len(cc.Return) > 0 {
			p.status = cc.Return[0]
		}
		return ErrController
	}
	if cc.Op == hci.OpReset {
		p.flow = hci.NewFlow(p.cfg.ACLBuffers) // the controller empties its buffers on reset
	}
	p.step++
	if int(p.step) == len(setup) {
		p.state = StateAdvertising
		return nil
	}
	return p.sendStep()
}

func (p *Peripheral) sendStep() error {
	var n int
	var err error
	switch setup[p.step] {
	case hci.OpReset:
		n, err = hci.PutReset(p.cmd[:])
	case hci.OpLESetAdvParams:
		n, err = hci.PutLESetAdvParams(p.cmd[:], &p.cfg.Adv)
	case hci.OpLESetAdvData:
		n, err = hci.PutLESetAdvData(p.cmd[:], false, p.cfg.AdvData)
	default:
		n, err = hci.PutLESetAdvEnable(p.cmd[:], true)
	}
	if err != nil {
		return err
	}
	return p.command(n)
}

// command sends the first n bytes of p.cmd.
func (p *Peripheral) command(n int) error {
	if !p.flow.CanSendCommand() {
		return ErrNoCredits
	}
	p.flow.TookCommand()
	return p.tr.WriteHCI(p.cmd[:n])
}

// connected prepares every layer for a new connection. The controller stops advertising by itself.
func (p *Peripheral) connected(c hci.Connection) {
	p.state, p.handle = StateConnected, c.Handle
	p.peer = smp.Address{Type: c.PeerAddrType & 1, Addr: c.PeerAddr}
	p.encrypted, p.saved, p.bonded = false, false, false
	p.server.Reset()
	p.client.Reset()
	p.pairing.Start(p.cfg.Own, p.peer)
	// The ATT server needs to know if a bond exists, to ask the peer to encrypt instead of to pair.
	if p.findBond() {
		clear(p.ltk[:])
	}
	p.server.SetSecurity(false, p.bonded)
}

// disconnected drops the state of the connection and advertises again.
func (p *Peripheral) disconnected() error {
	cccdErr := p.storeCCCDs()
	p.queue.Reset()
	p.reasm.Reset(p.handle)
	p.client.Reset()
	p.pairing.Start(p.cfg.Own, smp.Address{})
	p.encrypted = false
	p.state, p.step = StateStarting, uint8(len(setup)-1)
	if err := p.sendStep(); err != nil {
		return err
	}
	return cccdErr
}

// ltkRequest answers with the LTK of pairing on this connection, or of a stored bond with this peer.
// Secure Connections keys have EDIV and Rand zero (Core spec v5.4 Vol 3 Part H 2.4.4.1).
func (p *Peripheral) ltkRequest(r hci.LTKRequest) error {
	ok := false
	if r.Handle == p.handle && p.state == StateConnected && r.EDIV == 0 && r.Rand == 0 {
		ok = p.pairing.LTK(&p.ltk) || p.findBond()
	}
	var n int
	var err error
	if ok {
		n, err = hci.PutLELongTermKeyReply(p.cmd[:], r.Handle, &p.ltk)
	} else {
		n, err = hci.PutLELongTermKeyNegReply(p.cmd[:], r.Handle)
	}
	clear(p.ltk[:])
	if err != nil {
		return err
	}
	err = p.command(n)
	clear(p.cmd[:])
	return err
}

// findBond copies the LTK of the bond with the peer into p.ltk.
func (p *Peripheral) findBond() bool {
	found := false
	for i := 0; !found && p.bonds.Bond(i, &p.foundPeer, &p.found); i++ {
		b := &p.found
		found = smp.BondMatches(p.mem, b, p.foundPeer, p.peer)
		if found {
			p.ltk = b.LTK
			p.bondKey, p.bonded = p.foundPeer, true
			if b.HasPeerIRK {
				p.bondKey = b.PeerIdentity
			}
		}
	}
	p.found = smp.Bond{}
	return found
}

// saveBond stores the bond once pairing is complete, when both sides asked for bonding.
func (p *Peripheral) saveBond() error {
	if p.saved || p.pairing.State() != smp.StatePaired || !p.pairing.Bond(&p.found) {
		return nil
	}
	p.saved = true
	var err error
	if p.found.Bonded {
		err = p.bonds.SaveBond(&p.peer, &p.found)
		p.bondKey, p.bonded = p.peer, err == nil
		if p.found.HasPeerIRK {
			p.bondKey = p.found.PeerIdentity
		}
	}
	p.found = smp.Bond{}
	p.server.SetSecurity(p.encrypted, p.bonded)
	return err
}

// loadCCCDs gives a bonded peer its stored CCCD values after the link is encrypted.
func (p *Peripheral) loadCCCDs() error {
	n := p.server.CCCDCount()
	if p.cfg.CCCDs == nil || !p.bonded || n == 0 {
		return nil
	}
	buf := memory.MakeSlice[uint16](p.mem, n)
	if buf == nil {
		return ErrNoMemory
	}
	if p.cfg.CCCDs.LoadCCCDs(&p.bondKey, buf) {
		p.server.SetCCCDs(buf)
	}
	memory.FreeSlice(p.mem, buf)
	return nil
}

// storeCCCDs keeps the CCCD values of a bonded peer when it disconnects.
func (p *Peripheral) storeCCCDs() error {
	n := p.server.CCCDCount()
	if p.cfg.CCCDs == nil || !p.bonded || n == 0 {
		return nil
	}
	buf := memory.MakeSlice[uint16](p.mem, n)
	if buf == nil {
		return ErrNoMemory
	}
	p.server.CopyCCCDs(buf)
	err := p.cfg.CCCDs.SaveCCCDs(&p.bondKey, buf)
	memory.FreeSlice(p.mem, buf)
	return err
}

// frame routes one complete L2CAP frame by channel.
func (p *Peripheral) frame(cid uint16, payload []byte) error {
	switch cid {
	case l2cap.CIDATT:
		if !att.ForClient(payload) {
			err := p.server.Handle(payload)
			p.client.SetMTU(p.server.MTU())
			return err
		}
		done, err := p.client.Handle(payload)
		if done {
			p.server.SetMTU(p.client.MTU())
			p.cfg.Done.ProcedureDone(err)
			return nil
		}
		return err
	case l2cap.CIDSMP:
		if err := p.pairing.Handle(payload, p.now); err != nil {
			return err
		}
		return p.saveBond()
	case l2cap.CIDSignaling:
		s, err := l2cap.ParseSignal(payload)
		if err != nil || s.Code == l2cap.SigCommandReject || s.Code == l2cap.SigConnParamUpdateRsp {
			return nil
		}
		n, _ := l2cap.PutCommandReject(p.sig[:], s.Identifier)
		return p.send(l2cap.CIDSignaling, p.sig[:n])
	}
	return nil
}

func (p *Peripheral) send(cid uint16, payload []byte) error {
	return l2cap.Send(&p.link, p.mem, p.handle, cid, payload, p.cfg.MaxACL)
}

// link gives the layers the interfaces they need, so these methods are not part of the Peripheral API.
type link struct{ p *Peripheral }

func (l *link) SendATT(pdu []byte) error { return l.p.send(l2cap.CIDATT, pdu) }
func (l *link) SendSMP(pdu []byte) error { return l.p.send(l2cap.CIDSMP, pdu) }

// WriteACL sends a packet now when the queue is empty and a buffer is free, and queues it otherwise,
// so packets keep their order.
func (l *link) WriteACL(packet []byte) error {
	p := l.p
	if err := p.queue.Send(packet, &p.flow, p.tr); err != aclq.ErrNoCredits {
		return err
	}
	return ErrNoCredits
}

// drain sends queued packets while the controller has free buffers.
func (p *Peripheral) drain() error { return p.queue.Drain(&p.flow, p.tr) }

func (l *link) HandleFrame(_, cid uint16, payload []byte) {
	if err := l.p.frame(cid, payload); err != nil && l.p.frameErr == nil {
		l.p.frameErr = err
	}
}

type ignore struct{}

func (ignore) Notification(uint16, []byte, bool) {}
func (ignore) ProcedureDone(error)               {}
