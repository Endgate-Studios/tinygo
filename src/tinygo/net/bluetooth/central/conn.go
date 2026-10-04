package central

import (
	"encoding/binary"

	"tinygo/net/bluetooth/aclq"
	"tinygo/net/bluetooth/att"
	"tinygo/net/bluetooth/hci"
	"tinygo/net/bluetooth/l2cap"
	"tinygo/net/bluetooth/smp"
)

// ConnState is the state of the connection.
type ConnState uint8

const (
	ConnIdle ConnState = iota
	ConnConnecting
	ConnConnected
)

// ProcedureHandler receives the result of each GATT client procedure.
type ProcedureHandler interface {
	ProcedureDone(err error)
}

// ConnectionHandler learns when the connection starts and ends.
type ConnectionHandler interface {
	Connected(handle uint16, peer hci.Addr)
	Disconnected(reason uint8)
}

// ParamPolicy decides on a Connection Parameter Update Request of the peripheral. It sees only
// requests that are inside the ranges of the spec.
type ParamPolicy interface {
	AcceptParams(p l2cap.ConnParams) bool
}

// ConnParams are the parameters of the connection now. Interval is in 1.25 ms units, Timeout in 10 ms units.
type ConnParams struct {
	Interval, Latency, Timeout uint16
}

// Params returns the parameters of the connection now.
func (c *Central) Params() ConnParams { return c.params }

// ATT opcodes that the central answers as a minimal server (Core spec v5.4 Vol 3 Part F 3.4.8).
const (
	attError              = 0x01
	attMTUReq             = 0x02
	attMTURsp             = 0x03
	attConfirm            = 0x1e
	attCommand            = 0x40
	attRequestUnsupported = 0x06
)

// Client returns the GATT client of the connection.
func (c *Central) Client() *att.Client { return c.client }

// ConnState returns the state of the connection, and Handle its connection handle.
func (c *Central) ConnState() ConnState { return c.conn }
func (c *Central) Handle() uint16       { return c.handle }

// Connect starts a connection to the peer in p. Scanning must be off. The controller confirms
// with Command Status, and ConnectionHandler learns the result.
func (c *Central) Connect(p *hci.ConnParams) error {
	if c.conn != ConnIdle || c.state != StateIdle {
		return ErrBusy
	}
	n, err := hci.PutLECreateConnection(c.cmd[:], p)
	if err != nil {
		return err
	}
	if err := c.command(n); err != nil {
		return err
	}
	c.conn = ConnConnecting
	return nil
}

// Disconnect asks the controller to end the connection, with a reason such as 0x13 for remote
// user terminated. Disconnection Complete ends it.
func (c *Central) Disconnect(reason uint8) error {
	if c.conn != ConnConnected {
		return nil
	}
	n, err := hci.PutDisconnect(c.cmd[:], c.handle, reason)
	if err != nil {
		return err
	}
	return c.command(n)
}

// Tick runs the ATT transaction timeout.
func (c *Central) Tick(now uint32) {
	if done, err := c.client.Tick(now); done {
		c.cfg.Done.ProcedureDone(err)
	}
}

// Close frees the reassembly buffers and stops the client.
func (c *Central) Close() {
	c.queue.Reset()
	c.reasm.Close()
	c.client.Close()
	if c.pairing != nil {
		c.pairing.Close()
	}
}

// connEvent handles the events of connection setup and teardown, and reports whether e was one.
func (c *Central) connEvent(e hci.Event) (bool, error) {
	if ec, ok := e.EncryptionChange(); ok {
		return true, c.encryptionChange(ec)
	}
	if cs, ok := e.CommandStatus(); ok {
		if cs.Op == hci.OpLEEnableEncryption && cs.Status != 0 {
			c.encStarted, c.status = false, cs.Status
			return true, ErrController
		}
		if cs.Op != hci.OpLECreateConnection || c.conn != ConnConnecting {
			return true, nil
		}
		if cs.Status != 0 {
			c.conn, c.status = ConnIdle, cs.Status
			return true, ErrController
		}
		return true, nil
	}
	if d, ok := e.Disconnection(); ok {
		if c.conn == ConnConnected && d.Handle == c.handle {
			c.queue.Reset()
			c.reasm.Reset(c.handle)
			c.client.Reset()
			c.conn, c.mtu = ConnIdle, att.DefaultMTU
			c.encrypted, c.encStarted, c.saved = false, false, false
			if c.pairing != nil {
				c.pairing.Start(c.cfg.Own, smp.Address{})
			}
			c.cfg.Conn.Disconnected(d.Reason)
		}
		return true, nil
	}
	sub, params, ok := e.LEMeta()
	if ok && sub == hci.SubConnectionUpdate {
		u, err := hci.ParseConnectionUpdate(params)
		if err == nil && u.Status == 0 && c.conn == ConnConnected && u.Handle == c.handle {
			c.params = ConnParams{Interval: u.Interval, Latency: u.Latency, Timeout: u.Timeout}
		}
		return true, err
	}
	if !ok || sub != hci.SubConnectionComplete {
		return false, nil
	}
	conn, err := hci.ParseConnection(params)
	if err != nil || c.conn != ConnConnecting || conn.Role != 0 {
		return true, err
	}
	if conn.Status != 0 {
		c.conn, c.status = ConnIdle, conn.Status
		return true, ErrController
	}
	c.conn, c.handle, c.peer, c.mtu = ConnConnected, conn.Handle, conn.PeerAddr, att.DefaultMTU
	c.params = ConnParams{Interval: conn.Interval, Latency: conn.Latency, Timeout: conn.Timeout}
	c.peerID = smp.Address{Type: conn.PeerAddrType & 1, Addr: conn.PeerAddr}
	c.encrypted, c.encStarted, c.saved = false, false, false
	c.client.Reset()
	if c.pairing != nil {
		c.pairing.Start(c.cfg.Own, c.peerID)
	}
	c.cfg.Conn.Connected(conn.Handle, conn.PeerAddr)
	return true, nil
}

// acl gives the ACL data of the connection to the reassembler.
func (c *Central) acl(packet []byte) error {
	a, err := hci.ParseACL(packet)
	if err != nil || c.conn != ConnConnected || a.Handle != c.handle {
		return err
	}
	c.frameErr = nil
	if err := c.reasm.HandleACL(a); err != nil {
		return err
	}
	return c.frameErr
}

// frame routes one L2CAP frame by channel.
func (c *Central) frame(cid uint16, payload []byte) error {
	switch cid {
	case l2cap.CIDATT:
		if att.ForClient(payload) {
			done, err := c.client.Handle(payload)
			if done {
				c.cfg.Done.ProcedureDone(err)
				return nil
			}
			return err
		}
		return c.attRequest(payload)
	case l2cap.CIDSignaling:
		return c.signal(payload)
	case l2cap.CIDSMP:
		return c.smpFrame(payload)
	}
	return nil
}

// attRequest answers a request from the GATT client of the peer. The central has no database, so
// it answers the MTU exchange, which every server must (Vol 3 Part F 3.4.2), and rejects the rest.
func (c *Central) attRequest(p []byte) error {
	op := p[0]
	switch {
	case op&attCommand != 0 || op == attConfirm:
		return nil
	case op == attMTUReq && len(p) == 3:
		c.mtu = min(max(binary.LittleEndian.Uint16(p[1:]), att.DefaultMTU), max(c.cfg.MaxMTU, att.DefaultMTU))
		c.client.SetMTU(c.mtu)
		c.rsp[0] = attMTURsp
		binary.LittleEndian.PutUint16(c.rsp[1:], max(c.cfg.MaxMTU, att.DefaultMTU))
		return c.send(l2cap.CIDATT, c.rsp[:3])
	}
	c.rsp[0], c.rsp[1] = attError, op
	c.rsp[2], c.rsp[3], c.rsp[4] = 0, 0, attRequestUnsupported
	return c.send(l2cap.CIDATT, c.rsp[:5])
}

// signal answers LE signaling requests. A Connection Parameter Update Request is accepted when it is
// valid and the ParamPolicy agrees, and the controller then applies it (Vol 3 Part A 4.20 and 4.21).
func (c *Central) signal(p []byte) error {
	s, err := l2cap.ParseSignal(p)
	if err != nil {
		return nil
	}
	var n int
	switch s.Code {
	case l2cap.SigCommandReject, l2cap.SigConnParamUpdateRsp:
		return nil
	case l2cap.SigConnParamUpdateReq:
		return c.paramRequest(s)
	default:
		n, err = l2cap.PutCommandReject(c.rsp[:], s.Identifier)
	}
	if err != nil {
		return err
	}
	return c.send(l2cap.CIDSignaling, c.rsp[:n])
}

func (c *Central) send(cid uint16, payload []byte) error {
	return l2cap.Send(&c.link, c.mem, c.handle, cid, payload, c.cfg.MaxACL)
}

// link gives the layers the interfaces they need, so these methods are not part of the Central API.
type link struct{ c *Central }

func (l *link) SendATT(pdu []byte) error { return l.c.send(l2cap.CIDATT, pdu) }
func (l *link) SendSMP(pdu []byte) error { return l.c.send(l2cap.CIDSMP, pdu) }

// WriteACL sends a packet now, or queues it when the controller has no free buffer.
func (l *link) WriteACL(packet []byte) error {
	c := l.c
	if err := c.queue.Send(packet, &c.flow, c.tr); err != aclq.ErrNoCredits {
		return err
	}
	return ErrNoCredits
}

func (l *link) HandleFrame(_, cid uint16, payload []byte) {
	if err := l.c.frame(cid, payload); err != nil && l.c.frameErr == nil {
		l.c.frameErr = err
	}
}

type ignore struct{}

func (ignore) Notification(uint16, []byte, bool) {}
func (ignore) ProcedureDone(error)               {}
func (ignore) Connected(uint16, hci.Addr)        {}
func (ignore) Disconnected(uint8)                {}

// paramRequest answers a Connection Parameter Update Request, and on accept asks the controller to
// apply the parameters with HCI_LE_Connection_Update.
func (c *Central) paramRequest(s l2cap.Signal) error {
	p, err := l2cap.ParseConnParamUpdateReq(s)
	accept := err == nil && c.cfg.Params != nil && p.Valid() && c.cfg.Params.AcceptParams(p)
	result := uint16(l2cap.ParamsRejected)
	if accept {
		result = l2cap.ParamsAccepted
	}
	n, err := l2cap.PutConnParamUpdateRsp(c.rsp[:], s.Identifier, result)
	if err != nil {
		return err
	}
	if err := c.send(l2cap.CIDSignaling, c.rsp[:n]); err != nil || !accept {
		return err
	}
	u := hci.ConnUpdate{Handle: c.handle, IntervalMin: p.IntervalMin, IntervalMax: p.IntervalMax, Latency: p.Latency, SupervisionTimeout: p.Timeout}
	if n, err = hci.PutLEConnectionUpdate(c.cmd[:], &u); err != nil {
		return err
	}
	return c.command(n)
}
