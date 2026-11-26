package ip

import (
	"testing"
	"time"
)

var ntpServer = Addr{192, 168, 1, 123}

// ntpReply answers the request in frame with the transmit time at, mode,
// stratum and the origin from the request, or origin when it is not nil.
func ntpReply(t *testing.T, frame []byte, at time.Time, mode, stratum byte, origin []byte) []byte {
	t.Helper()
	req := frame[14+20+8:]
	if len(req) < ntpLen || req[0] != 0x23 {
		t.Fatalf("request %x", req)
	}
	sport := be16(frame[14+20:])
	m := make([]byte, ntpLen)
	m[0], m[1] = 4<<3|mode, stratum
	if origin == nil {
		origin = req[40:48]
	}
	copy(m[24:32], origin)
	secs := uint32(at.Unix() + ntpUnix)
	frac := uint32(uint64(at.Nanosecond()) << 32 / 1e9)
	put32(m[32:], secs)
	put32(m[36:], frac)
	put32(m[40:], secs)
	put32(m[44:], frac)
	return ethernet(myMAC, peerMAC, etherIPv4, udpPacket(ntpServer, myIP, ntpPort, sport, m))
}

func TestSNTP(t *testing.T) {
	s, l, _ := newTestStack(t)
	s.arp.update(ntpServer, peerMAC)
	at := time.Date(2026, 10, 5, 12, 0, 0, 500_000_000, time.UTC)
	type result struct {
		t   time.Time
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := s.SNTP(ntpServer, 3*time.Second)
		done <- result{got, err}
	}()
	f := l.next(t)
	// An answer with another origin is not for this request: it is ignored.
	s.Input(ntpReply(t, f, at.Add(time.Hour), 4, 2, make([]byte, 8)))
	s.Input(ntpReply(t, f, at, 4, 2, nil))
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	if d := r.t.Sub(at); d < 0 || d > 100*time.Millisecond {
		t.Errorf("time %v, want %v plus a short round trip", r.t, at)
	}

	go func() {
		got, err := s.SNTP(ntpServer, 3*time.Second)
		done <- result{got, err}
	}()
	s.Input(ntpReply(t, l.next(t), at, 4, 0, nil))
	if r := <-done; r.err != ErrNTPKiss {
		t.Errorf("stratum 0: %v, want ErrNTPKiss", r.err)
	}
}

func TestNTPTime(t *testing.T) {
	// 2036-02-07 06:28:16 UTC is second 0 of era 1.
	b := []byte{0, 0, 0, 1, 0x80, 0, 0, 0}
	if got, want := ntpTime(b), time.Date(2036, 2, 7, 6, 28, 17, 500_000_000, time.UTC); !got.Equal(want) {
		t.Errorf("era 1: %v, want %v", got, want)
	}
	b = []byte{0xeb, 0, 0, 0, 0, 0, 0, 0}
	if got := ntpTime(b); got.Year() != 2024 {
		t.Errorf("era 0: %v", got)
	}
	if !ntpTime(make([]byte, 8)).IsZero() {
		t.Error("a zero timestamp is not the zero time")
	}
	if _, err := ntpAnswer(make([]byte, ntpLen), 0); err != ErrNTPFormat {
		t.Errorf("mode 0: %v, want ErrNTPFormat", err)
	}
}
