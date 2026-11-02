// Package tsf pairs a reading of the Wi-Fi TSF (Timing Synchronization
// Function) timer with readings of a local clock. Package radio makes the
// readings on the ESP32-S3. This package has only the arithmetic, so it runs
// on the host too.
//
// The TSF counts microseconds. A station loads the TSF of its access point
// from the timestamp of received beacons and probe responses (IEEE 802.11-2020
// clause 11.1.3.1), so all stations of one access point have nearly the same
// TSF.
package tsf

// Reading is a TSF value and the local clock just before and just after it.
// The TSF was read at some time between Before and After.
type Reading struct {
	TSF    int64 // the TSF in microseconds
	Before int64 // the local clock in nanoseconds before the TSF read
	After  int64 // the local clock in nanoseconds after the TSF read
}

// Local gives the local clock in nanoseconds at the middle of the read. The
// error of Local is at most Window()/2, plus the 1 µs resolution of the TSF.
func (r Reading) Local() int64 {
	return r.Before + (r.After-r.Before)/2
}

// Window gives the time in nanoseconds from Before to After. A smaller window
// gives a more exact pair.
func (r Reading) Window() int64 {
	return r.After - r.Before
}

// Offset gives the TSF minus the local clock, in nanoseconds, at Local. For a
// local time t in nanoseconds, the TSF is about (t + Offset()) / 1000.
func (r Reading) Offset() int64 {
	return r.TSF*1000 - r.Local()
}

// Best calls read n times and gives the reading with the smallest window, as
// the minimum filter of NTP. An n below 1 is 1. It stops at the first error of
// read and gives that error.
func Best(n int, read func() (Reading, error)) (Reading, error) {
	best, err := read()
	if err != nil {
		return Reading{}, err
	}
	for i := 1; i < n; i++ {
		r, err := read()
		if err != nil {
			return Reading{}, err
		}
		if r.Window() < best.Window() {
			best = r
		}
	}
	return best, nil
}
