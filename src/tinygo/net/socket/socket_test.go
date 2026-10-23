//go:build linux && (amd64 || arm64)

package socket

import (
	"testing"
	"time"
)

func TestPutPort(t *testing.T) {
	for _, tc := range []struct {
		port uint16
		want string
	}{{0, "0"}, {7, "7"}, {443, "443"}, {65535, "65535"}} {
		var s [6]byte
		putPort(&s, tc.port)
		n := 0
		for n < len(s) && s[n] != 0 {
			n++
		}
		if got := string(s[:n]); got != tc.want || n == len(s) {
			t.Errorf("putPort(%d) = %q, want %q with a NUL", tc.port, got, tc.want)
		}
	}
}

func TestDialErrors(t *testing.T) {
	var c Conn
	long := string(make([]byte, maxHost))
	if err := c.Dial(long, 443, time.Second); err != ErrHost {
		t.Errorf("long host: %v, want %v", err, ErrHost)
	}
	// Nothing listens on port 1 of the loopback address.
	if err := c.Dial("127.0.0.1", 1, time.Second); err != ErrConnect {
		t.Errorf("closed port: %v, want %v", err, ErrConnect)
	}
	if _, err := c.Read(make([]byte, 1)); err != ErrClosed {
		t.Errorf("read without a connection: %v, want %v", err, ErrClosed)
	}
}
