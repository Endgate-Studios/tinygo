// Package https is a small HTTPS client: DNS and TCP with tinygo/net/ip,
// TLS 1.3, or 1.2, with tinygo/net/tls and HTTP/1.1 with tinygo/net/web. Each request
// has its own connection, and its memory is free again after it.
package https

import (
	"errors"
	"io"
	"time"

	"runtime/memory"
	"tinygo/net/ip"
	"tinygo/net/tls"
	"tinygo/net/web"
)

// ErrHost tells that the host has a bad port.
var ErrHost = errors.New("https: bad port in the host")

// DefaultBufSize is the size of the buffer of a request when Client.Buf is
// nil.
const DefaultBufSize = 4096

// Client sends HTTPS requests. Set the fields, then use it as a value.
type Client struct {
	Stack *ip.Stack
	// Mem gives the TLS buffers of a request, about 30 KB during the
	// handshake.
	Mem tls.Memory
	// Rand gives the random values of TLS, for example the hardware random
	// number generator.
	Rand io.Reader
	// TLS is the configuration of each connection, with the trusted keys.
	// ServerName is the host of the request. With TLS.Session, the next
	// request to the same host resumes the session.
	TLS tls.Config
	// Now gives the current time for the validity check of the
	// certificates, for example from SNTP. When it is nil, TLS.Time stays.
	Now func() time.Time
	// Buf holds the request head and the response head, then the body as it
	// arrives. Response heads are often 1 to 2 KB, and some 5 KB. When Buf
	// is nil, each request takes DefaultBufSize bytes from Mem.
	Buf []byte
	// TCPBuffer is the size of the TCP buffers. The default is 4096.
	TCPBuffer int
	// Timeout is for the DNS lookup, the TCP connection, the handshake and
	// each read. The default is 10 seconds.
	Timeout time.Duration
	// Dial, when not nil, makes the connection instead of Stack, for example
	// with tinygo/net/socket. Do closes a connection that is an io.Closer.
	Dial func(host string, port uint16, timeout time.Duration) (tls.RawConn, error)
}

// Get sends GET path to host and copies the body of the response to body.
// host can have a port, for example "example.com:8443". It gives the status
// code.
func (c *Client) Get(host, path string, body io.Writer) (int, error) {
	return c.Do("GET", host, path, "", nil, body)
}

// Post sends data with the content type to path on host, and copies the
// body of the response to body. It gives the status code.
func (c *Client) Post(host, path, contentType string, data []byte, body io.Writer) (int, error) {
	return c.Do("POST", host, path, "Content-Type: "+contentType+"\r\n", data, body)
}

// Do sends a request as web.Do does, on a new TLS connection to host.
func (c *Client) Do(method, host, path, header string, data []byte, body io.Writer) (int, error) {
	status, _, err := c.do(method, host, path, header, data, body, nil)
	return status, err
}

// DoRedirect is Do, and for a redirect gives its Location in location, as web.DoRedirect does.
func (c *Client) DoRedirect(method, host, path, header string, data []byte, body io.Writer, location []byte) (status, n int, err error) {
	return c.do(method, host, path, header, data, body, location)
}

func (c *Client) do(method, host, path, header string, data []byte, body io.Writer, location []byte) (int, int, error) {
	name, port, err := splitHost(host)
	if err != nil {
		return 0, 0, err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	raw, err := c.connect(name, port, timeout)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if cl, ok := raw.(io.Closer); ok {
			cl.Close()
		}
	}()
	cfg := c.TLS
	cfg.ServerName = name
	if c.Now != nil {
		cfg.Time = c.Now()
	}
	conn := tls.Client(raw, c.Mem, c.Rand, &cfg)
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(timeout))
	if err := conn.Handshake(); err != nil {
		return 0, 0, err
	}
	buf := c.Buf
	if buf == nil {
		if buf = memory.MakeSlice[byte](c.Mem, DefaultBufSize); buf == nil {
			return 0, 0, tls.ErrNoMemory
		}
		defer memory.FreeSlice(c.Mem, buf)
	}
	return web.DoRedirect(&deadlineConn{conn, timeout}, method, name, path, header, data, buf, body, location)
}

func (c *Client) connect(name string, port uint16, timeout time.Duration) (tls.RawConn, error) {
	if c.Dial != nil {
		return c.Dial(name, port, timeout)
	}
	addr, err := c.Stack.LookupHost(name, timeout)
	if err != nil {
		return nil, err
	}
	size := c.TCPBuffer
	if size == 0 {
		size = 4096
	}
	return c.Stack.DialTCP(addr, port, size, timeout)
}

// deadlineConn sets the read deadline again before each read, so a long
// body can take more time than one timeout.
type deadlineConn struct {
	*tls.Conn
	timeout time.Duration
}

func (d *deadlineConn) Read(b []byte) (int, error) {
	d.SetReadDeadline(time.Now().Add(d.timeout))
	return d.Conn.Read(b)
}

// splitHost cuts an optional port from host. The default is 443.
func splitHost(host string) (string, uint16, error) {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			p := host[i+1:]
			if len(p) == 0 || len(p) > 5 {
				return "", 0, ErrHost
			}
			v := 0
			for _, ch := range []byte(p) {
				if ch < '0' || ch > '9' {
					return "", 0, ErrHost
				}
				v = v*10 + int(ch-'0')
			}
			if v == 0 || v > 65535 {
				return "", 0, ErrHost
			}
			return host[:i], uint16(v), nil
		}
	}
	return host, 443, nil
}
