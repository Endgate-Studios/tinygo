//go:build linux && (amd64 || arm64)

// Package socket is a TCP connection through libc sockets, for tinygo/net/tls
// and tinygo/net/web on Linux. It does not allocate.
package socket

import (
	"io"
	"time"
	"unsafe"
)

// Error is an error of a socket.
type Error string

func (e Error) Error() string { return string(e) }

// Timeout reports whether the error is a deadline or a timeout, as the Timeout method of net.Error.
func (e Error) Timeout() bool { return e == ErrDeadline || e == ErrTimeout }

const (
	ErrHost     Error = "socket: host name too long"
	ErrLookup   Error = "socket: host not found"
	ErrConnect  Error = "socket: connection failed"
	ErrTimeout  Error = "socket: connection timed out"
	ErrDeadline Error = "socket: read deadline passed"
	ErrClosed   Error = "socket: connection closed"
	ErrIO       Error = "socket: send or receive failed"
)

// The values are the same in glibc and musl on amd64 and arm64.
const (
	sockStream   = 1
	sockNonblock = 0o4000
	sockCloexec  = 0o2000000
	ipprotoTCP   = 6
	solSocket    = 1
	soError      = 4
	msgNosignal  = 0x4000
	pollIn       = 0x1
	pollOut      = 0x4
	eintr        = 4
	eagain       = 11
	einprogress  = 115
)

// maxHost is the longest host name with the NUL.
const maxHost = 256

// Conn is a TCP connection that satisfies tls.RawConn. Put it in memory that
// you own, for example a global, and use Dial.
type Conn struct {
	fd       int32
	open     bool
	timeout  time.Duration
	deadline time.Time
	// res is the getaddrinfo result. A local is allocated on the heap, because
	// LLVM merges &res with the next fields in the loop.
	res *addrinfo
	// Errno is the errno of the last failure, for the logs.
	Errno int32
}

// addrinfo is struct addrinfo. The layout is the same in glibc and musl.
type addrinfo struct {
	flags, family, socktype, protocol int32
	addrlen                           uint32
	addr                              unsafe.Pointer
	canonname                         *byte
	next                              *addrinfo
}

type pollfd struct {
	fd              int32
	events, revents int16
}

// Dial connects c to host on port. host is a name or an IPv4 or IPv6 address.
// timeout is for the connection and for each Write.
//
//go:noheap
func (c *Conn) Dial(host string, port uint16, timeout time.Duration) error {
	*c = Conn{fd: -1, timeout: timeout}
	var node [maxHost]byte
	if len(host) >= len(node) {
		return ErrHost
	}
	copy(node[:], host)
	var service [6]byte
	putPort(&service, port)
	hints := addrinfo{socktype: sockStream, protocol: ipprotoTCP}
	if libc_getaddrinfo(&node[0], &service[0], &hints, &c.res) != 0 {
		return ErrLookup
	}
	deadline := time.Now().Add(timeout)
	var err error = ErrLookup
	for ai := c.res; ai != nil; ai = ai.next {
		if err = c.connect(ai, deadline); err == nil {
			break
		}
	}
	libc_freeaddrinfo(c.res)
	c.res = nil
	return err
}

//go:noheap
func (c *Conn) connect(ai *addrinfo, deadline time.Time) error {
	fd := libc_socket(ai.family, ai.socktype|sockNonblock|sockCloexec, ai.protocol)
	if fd < 0 {
		c.Errno = *libc_errno_location()
		return ErrConnect
	}
	if libc_connect(fd, ai.addr, ai.addrlen) != 0 {
		if errno := *libc_errno_location(); errno != einprogress {
			c.Errno = errno
			libc_close(fd)
			return ErrConnect
		}
		if err := wait(fd, pollOut, deadline, ErrTimeout); err != nil {
			libc_close(fd)
			return err
		}
		var soErr int32
		n := uint32(unsafe.Sizeof(soErr))
		if libc_getsockopt(fd, solSocket, soError, unsafe.Pointer(&soErr), &n) != 0 || soErr != 0 {
			c.Errno = soErr
			libc_close(fd)
			return ErrConnect
		}
	}
	c.fd, c.open = fd, true
	return nil
}

// SetReadDeadline sets the deadline of Read. The zero time is no deadline.
func (c *Conn) SetReadDeadline(t time.Time) { c.deadline = t }

// Read reads into b. It gives io.EOF when the server closes.
//
//go:noheap
func (c *Conn) Read(b []byte) (int, error) {
	if !c.open {
		return 0, ErrClosed
	}
	if len(b) == 0 {
		return 0, nil
	}
	for {
		n := libc_recv(c.fd, unsafe.Pointer(&b[0]), uintptr(len(b)), 0)
		if n > 0 {
			return n, nil
		}
		if n == 0 {
			return 0, io.EOF
		}
		switch errno := *libc_errno_location(); errno {
		case eintr:
		case eagain:
			if err := wait(c.fd, pollIn, c.deadline, ErrDeadline); err != nil {
				return 0, err
			}
		default:
			c.Errno = errno
			return 0, ErrIO
		}
	}
}

// Write writes all of b, or stops after the timeout of Dial.
//
//go:noheap
func (c *Conn) Write(b []byte) (int, error) {
	if !c.open {
		return 0, ErrClosed
	}
	deadline := time.Now().Add(c.timeout)
	sent := 0
	for sent < len(b) {
		// MSG_NOSIGNAL makes a closed connection fail the send. SIGPIPE would
		// stop the process (send(2)).
		n := libc_send(c.fd, unsafe.Pointer(&b[sent]), uintptr(len(b)-sent), msgNosignal)
		if n >= 0 {
			sent += n
			continue
		}
		switch errno := *libc_errno_location(); errno {
		case eintr:
		case eagain:
			if err := wait(c.fd, pollOut, deadline, ErrTimeout); err != nil {
				return sent, err
			}
		default:
			c.Errno = errno
			return sent, ErrIO
		}
	}
	return sent, nil
}

// Close closes the connection.
func (c *Conn) Close() error {
	if !c.open {
		return nil
	}
	c.open = false
	libc_close(c.fd)
	return nil
}

// wait waits until fd is ready for events, or gives late at the deadline. The
// zero deadline has no end. late is an error, because an Error variable in an
// interface allocates.
//
//go:noheap
func wait(fd int32, events int16, deadline time.Time, late error) error {
	for {
		ms := int32(-1)
		if !deadline.IsZero() {
			d := time.Until(deadline)
			if d <= 0 {
				return late
			}
			ms = int32(min(d.Milliseconds()+1, 1<<31-1))
		}
		p := pollfd{fd: fd, events: events}
		switch n := libc_poll(&p, 1, ms); {
		case n > 0:
			// Ready. The next call gives an error of the socket.
			return nil
		case n == 0:
			return late
		case *libc_errno_location() != eintr:
			return ErrIO
		}
	}
}

// putPort writes port in decimal into s, with a NUL at the end.
func putPort(s *[6]byte, port uint16) {
	var d [5]byte
	i := len(d)
	for {
		i--
		d[i] = byte('0' + port%10)
		port /= 10
		if port == 0 {
			break
		}
	}
	copy(s[:], d[i:])
}

// int getaddrinfo(const char *node, const char *service,
//                 const struct addrinfo *hints, struct addrinfo **res);
//
//export getaddrinfo
//go:noescape
func libc_getaddrinfo(node, service *byte, hints *addrinfo, res **addrinfo) int32

// void freeaddrinfo(struct addrinfo *res);
//
//export freeaddrinfo
//go:noescape
func libc_freeaddrinfo(res *addrinfo)

// int socket(int domain, int type, int protocol);
//
//export socket
func libc_socket(domain, typ, protocol int32) int32

// int connect(int fd, const struct sockaddr *addr, socklen_t len);
//
//export connect
//go:noescape
func libc_connect(fd int32, addr unsafe.Pointer, addrlen uint32) int32

// int getsockopt(int fd, int level, int name, void *val, socklen_t *len);
//
//export getsockopt
//go:noescape
func libc_getsockopt(fd, level, name int32, val unsafe.Pointer, n *uint32) int32

// int poll(struct pollfd *fds, nfds_t nfds, int timeout);
//
//export poll
//go:noescape
func libc_poll(fds *pollfd, nfds uintptr, timeout int32) int32

// ssize_t recv(int fd, void *buf, size_t n, int flags);
//
//export recv
//go:noescape
func libc_recv(fd int32, buf unsafe.Pointer, n uintptr, flags int32) int

// ssize_t send(int fd, const void *buf, size_t n, int flags);
//
//export send
//go:noescape
func libc_send(fd int32, buf unsafe.Pointer, n uintptr, flags int32) int

// int close(int fd);
//
//export close
func libc_close(fd int32) int32

// int *__errno_location(void);
//
//export __errno_location
func libc_errno_location() *int32
