// Package netdev connects an ip.Stack to the TinyGo net package. After Use,
// net.Dial, net.Listen and net/http run on the stack.
package netdev

import (
	"errors"
	"net/netip"
	"os"
	"sync"
	"time"
	_ "unsafe" // for go:linkname

	_ "net" // Use links to net.useNetdev
	"tinygo/net/ip"
)

// Values of the socket calls of src/net/netdev.go.
const (
	afINET     = 0x2
	sockStream = 0x1
	sockDgram  = 0x2
	protoTLS   = 0xFE

	maxSockets = 16

	// DialTimeout is how long Connect waits for a TCP connection.
	DialTimeout = 10 * time.Second

	// LookupTimeout is how long GetHostByName waits for the DNS server.
	LookupTimeout = 5 * time.Second
)

var (
	ErrBadSocket   = errors.New("netdev: bad socket")
	ErrNoSockets   = errors.New("netdev: no free socket")
	ErrUnsupported = errors.New("netdev: not supported")
)

// netdever is the interface of src/net/netdev.go.
type netdever interface {
	GetHostByName(name string) (netip.Addr, error)
	Addr() (netip.Addr, error)
	Socket(domain int, stype int, protocol int) (sockfd int, _ error)
	Bind(sockfd int, ip netip.AddrPort) error
	Connect(sockfd int, host string, ip netip.AddrPort) error
	Listen(sockfd int, backlog int) error
	Accept(sockfd int) (int, netip.AddrPort, error)
	Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Close(sockfd int) error
	SetSockOpt(sockfd int, level int, opt int, value interface{}) error
}

//go:linkname useNetdev net.useNetdev
func useNetdev(dev netdever)

type socket struct {
	used  bool
	stype int
	port  uint16 // local port from Bind
	tcp   *ip.TCPConn
	lis   *ip.TCPListener
	udp   *ip.UDPConn
	peer  ip.Addr // of a connected UDP socket, or the last sender
	pport uint16
	fixed bool // Connect set the peer of the UDP socket
}

// Device is a netdev on an ip.Stack.
type Device struct {
	s       *ip.Stack
	bufSize int
	mu      sync.Mutex
	socks   [maxSockets]socket
}

// New makes a device on s. Each TCP connection and UDP socket has rings of
// bufSize bytes, or the default of the stack when bufSize is 0.
func New(s *ip.Stack, bufSize int) *Device {
	return &Device{s: s, bufSize: bufSize}
}

// Use makes d the netdev of the net package.
func (d *Device) Use() { useNetdev(d) }

func (d *Device) get(fd int) (*socket, error) {
	if fd < 0 || fd >= maxSockets || !d.socks[fd].used {
		return nil, ErrBadSocket
	}
	return &d.socks[fd], nil
}

func (d *Device) GetHostByName(name string) (netip.Addr, error) {
	a, err := d.s.LookupHost(name, LookupTimeout)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.AddrFrom4(a), nil
}

func (d *Device) Addr() (netip.Addr, error) {
	a := d.s.Config().Addr
	if a.IsZero() {
		return netip.Addr{}, ip.ErrNoAddress
	}
	return netip.AddrFrom4(a), nil
}

func (d *Device) Socket(domain int, stype int, protocol int) (int, error) {
	if domain != afINET || (stype != sockStream && stype != sockDgram) || protocol == protoTLS {
		return -1, ErrUnsupported
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.socks {
		if !d.socks[i].used {
			d.socks[i] = socket{used: true, stype: stype}
			return i, nil
		}
	}
	return -1, ErrNoSockets
}

// Bind sets the local port. A UDP socket opens there, because net calls only
// Bind for net.ListenUDP.
func (d *Device) Bind(fd int, a netip.AddrPort) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	so, err := d.get(fd)
	if err != nil {
		return err
	}
	so.port = a.Port()
	if so.stype == sockDgram {
		so.udp, err = d.s.ListenUDP(so.port, d.udpBuf())
	}
	return err
}

func (d *Device) udpBuf() int {
	if d.bufSize == 0 {
		return ip.DefaultTCPBuffer
	}
	return d.bufSize
}

func (d *Device) Connect(fd int, host string, a netip.AddrPort) error {
	d.mu.Lock()
	so, err := d.get(fd)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	addr := ip.Addr(a.Addr().As4())
	if !a.Addr().IsValid() {
		if host == "" {
			return ErrBadSocket
		}
		if addr, err = d.s.LookupHost(host, LookupTimeout); err != nil {
			return err
		}
	}
	if so.stype == sockDgram {
		d.mu.Lock()
		defer d.mu.Unlock()
		if so.udp == nil {
			if so.udp, err = d.s.ListenUDP(so.port, d.udpBuf()); err != nil {
				return err
			}
		}
		so.peer, so.pport, so.fixed = addr, a.Port(), true
		return nil
	}
	c, err := d.s.DialTCP(addr, a.Port(), d.bufSize, DialTimeout)
	if err != nil {
		return err
	}
	d.mu.Lock()
	so.tcp = c
	d.mu.Unlock()
	return nil
}

func (d *Device) Listen(fd int, backlog int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	so, err := d.get(fd)
	if err != nil {
		return err
	}
	if so.stype != sockStream {
		return ErrUnsupported
	}
	so.lis, err = d.s.ListenTCP(so.port, d.bufSize)
	return err
}

func (d *Device) Accept(fd int) (int, netip.AddrPort, error) {
	d.mu.Lock()
	so, err := d.get(fd)
	d.mu.Unlock()
	if err != nil || so.lis == nil {
		return -1, netip.AddrPort{}, ErrBadSocket
	}
	c, err := so.lis.Accept()
	if err != nil {
		return -1, netip.AddrPort{}, err
	}
	nfd, err := d.Socket(afINET, sockStream, 0)
	if err != nil {
		c.Close()
		return -1, netip.AddrPort{}, err
	}
	d.mu.Lock()
	d.socks[nfd].tcp = c
	d.mu.Unlock()
	ra, rp := c.RemoteAddr()
	return nfd, netip.AddrPortFrom(netip.AddrFrom4(ra), rp), nil
}

// Send writes to a TCP connection, or sends one datagram to the peer of a UDP
// socket. An unconnected UDP socket answers the last sender.
func (d *Device) Send(fd int, buf []byte, flags int, deadline time.Time) (int, error) {
	d.mu.Lock()
	so, err := d.get(fd)
	d.mu.Unlock()
	if err != nil {
		return -1, err
	}
	switch {
	case so.tcp != nil:
		so.tcp.SetWriteDeadline(deadline)
		n, err := so.tcp.Write(buf)
		return n, mapError(err)
	case so.udp != nil:
		d.mu.Lock()
		peer, port := so.peer, so.pport
		d.mu.Unlock()
		if err := so.udp.WriteTo(buf, peer, port); err != nil {
			return -1, mapError(err)
		}
		return len(buf), nil
	}
	return -1, ErrBadSocket
}

func (d *Device) Recv(fd int, buf []byte, flags int, deadline time.Time) (int, error) {
	d.mu.Lock()
	so, err := d.get(fd)
	d.mu.Unlock()
	if err != nil {
		return -1, err
	}
	switch {
	case so.tcp != nil:
		so.tcp.SetReadDeadline(deadline)
		n, err := so.tcp.Read(buf)
		return n, mapError(err)
	case so.udp != nil:
		so.udp.SetReadDeadline(deadline)
		n, from, port, err := so.udp.ReadFrom(buf)
		if err != nil {
			return -1, mapError(err)
		}
		d.mu.Lock()
		if !so.fixed {
			so.peer, so.pport = from, port
		}
		d.mu.Unlock()
		return n, nil
	}
	return -1, ErrBadSocket
}

func (d *Device) Close(fd int) error {
	d.mu.Lock()
	so, err := d.get(fd)
	if err != nil {
		d.mu.Unlock()
		return err
	}
	tcp, lis, udp := so.tcp, so.lis, so.udp
	*so = socket{}
	d.mu.Unlock()
	switch {
	case tcp != nil:
		return tcp.Close()
	case lis != nil:
		return lis.Close()
	case udp != nil:
		return udp.Close()
	}
	return nil
}

// SetSockOpt accepts the keep-alive and linger options of net and ignores
// them.
func (d *Device) SetSockOpt(fd int, level int, opt int, value interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.get(fd)
	return err
}

// mapError gives the error that net expects for a passed deadline.
func mapError(err error) error {
	if err == ip.ErrDeadline {
		return os.ErrDeadlineExceeded
	}
	return err
}
