package mqtt

import (
	"io"
	"testing"
	"time"
)

// streamConn reads a fixed byte stream and drops the writes.
type streamConn struct{ in []byte }

func (s *streamConn) Read(b []byte) (int, error) {
	if len(s.in) == 0 {
		return 0, io.EOF
	}
	n := copy(b, s.in)
	s.in = s.in[n:]
	return n, nil
}

func (s *streamConn) Write(b []byte) (int, error) { return len(b), nil }
func (s *streamConn) SetReadDeadline(time.Time)   {}

// FuzzReceive gives a stream of packets from the broker to the client.
func FuzzReceive(f *testing.F) {
	f.Add([]byte{0x20, 2, 0, 0, 0x90, 3, 0, 1, 1, 0x32, 9, 0, 3, 'c', 'm', 'd', 0, 7, 'o', 'n', 0xd0, 0})
	f.Add([]byte{0x30, 0xc8, 0x01, 0, 1, 't'})
	f.Fuzz(func(t *testing.T, stream []byte) {
		c := New(&streamConn{in: stream}, make([]byte, 64), make([]byte, 64), func(m *Message) {
			if len(m.Topic)+len(m.Payload) > 64 {
				t.Fatal("message is larger than the receive buffer")
			}
		})
		for i := 0; i < 16; i++ {
			if c.Poll(time.Millisecond) != nil {
				return
			}
		}
	})
}
