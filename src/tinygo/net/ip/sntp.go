package ip

import "time"

// An SNTP client (RFC 4330, the client of RFC 5905).

const (
	ntpPort  = 123
	ntpLen   = 48
	ntpTries = 3
	// ntpUnix is the number of seconds from 1900 to 1970.
	ntpUnix = 2208988800
)

const (
	ErrNTPFormat Error = "ip: bad NTP answer"
	ErrNTPKiss   Error = "ip: NTP server sent a kiss-of-death"
)

// SNTP asks the NTP server for the time. It gives the transmit time of the
// server plus half the round trip, and it tries 3 times within timeout.
func (s *Stack) SNTP(server Addr, timeout time.Duration) (time.Time, error) {
	c, err := s.ListenUDP(0, 256)
	if err != nil {
		return time.Time{}, err
	}
	defer c.Close()
	var req [ntpLen]byte
	var buf [ntpLen + 16]byte
	per := timeout / ntpTries
	for try := 0; try < ntpTries; try++ {
		// LI 0, version 4, mode 3 (client). The transmit timestamp is a random
		// value, which the server sends back as the origin timestamp.
		clear(req[:])
		req[0] = 0x23
		s.mu.Lock()
		for i := 40; i < ntpLen; i += 4 {
			s.isnSeed = s.isnSeed*1103515245 + uint32(time.Now().UnixNano())
			put32(req[i:], s.isnSeed)
		}
		s.mu.Unlock()
		t1 := time.Now()
		if err := c.WriteTo(req[:], server, ntpPort); err != nil {
			return time.Time{}, err
		}
		c.SetReadDeadline(t1.Add(per))
		for {
			n, from, port, err := c.ReadFrom(buf[:])
			if err == ErrDeadline {
				break
			}
			if err != nil {
				return time.Time{}, err
			}
			if from != server || port != ntpPort || n < ntpLen || !sameBytes(buf[24:32], req[40:48]) {
				continue // not the answer to this request
			}
			return ntpAnswer(buf[:n], time.Since(t1))
		}
	}
	return time.Time{}, ErrTimeout
}

// ntpAnswer checks an answer and gives its time, with half of the round trip
// rtt minus the time that the server held the request.
func ntpAnswer(m []byte, rtt time.Duration) (time.Time, error) {
	mode, version, stratum := m[0]&7, m[0]>>3&7, m[1]
	if mode != 4 || version < 3 || version > 4 || stratum >= 16 {
		return time.Time{}, ErrNTPFormat
	}
	if stratum == 0 {
		return time.Time{}, ErrNTPKiss // RFC 4330 section 8
	}
	t2, t3 := ntpTime(m[32:40]), ntpTime(m[40:48])
	if t3.IsZero() {
		return time.Time{}, ErrNTPFormat
	}
	delay := rtt - t3.Sub(t2)
	if delay < 0 {
		delay = 0
	}
	return t3.Add(delay / 2), nil
}

// ntpTime converts a 64-bit NTP timestamp. A first bit of 0 means a time
// after 2036, in the next era (RFC 4330 section 3).
func ntpTime(b []byte) time.Time {
	secs, frac := int64(be32(b)), int64(be32(b[4:]))
	if secs == 0 && frac == 0 {
		return time.Time{}
	}
	if secs < 1<<31 {
		secs += 1 << 32
	}
	return time.Unix(secs-ntpUnix, frac*1e9>>32).UTC()
}

func sameBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
