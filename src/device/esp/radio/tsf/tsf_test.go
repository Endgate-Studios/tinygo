package tsf

import "testing"

type testError string

func (e testError) Error() string { return string(e) }

const errRead testError = "read failed"

func TestReading(t *testing.T) {
	r := Reading{TSF: 5_000_000, Before: 1_000_000_000, After: 1_000_001_500}
	if got := r.Window(); got != 1500 {
		t.Errorf("Window = %d, want 1500", got)
	}
	if got := r.Local(); got != 1_000_000_750 {
		t.Errorf("Local = %d, want 1000000750", got)
	}
	// 5 s of TSF at 1.00000075 s of local time.
	if got := r.Offset(); got != 5_000_000_000-1_000_000_750 {
		t.Errorf("Offset = %d, want %d", got, 5_000_000_000-1_000_000_750)
	}
	// The TSF from the offset at the local time of the reading.
	if got := (r.Local() + r.Offset()) / 1000; got != r.TSF {
		t.Errorf("TSF from Offset = %d, want %d", got, r.TSF)
	}
}

func TestReadingNegativeClock(t *testing.T) {
	// Local is a midpoint, also when the clock values are negative.
	r := Reading{TSF: 1, Before: -1001, After: -1}
	if got := r.Local(); got != -501 {
		t.Errorf("Local = %d, want -501", got)
	}
}

func TestBestSmallestWindow(t *testing.T) {
	readings := []Reading{
		{TSF: 10, Before: 0, After: 3000},
		{TSF: 20, Before: 10_000, After: 10_900},
		{TSF: 30, Before: 20_000, After: 22_000},
		{TSF: 40, Before: 30_000, After: 30_900}, // same window: the first stays
	}
	i := 0
	read := func() (Reading, error) {
		r := readings[i]
		i++
		return r, nil
	}
	got, err := Best(len(readings), read)
	if err != nil {
		t.Fatal(err)
	}
	if got != readings[1] {
		t.Errorf("Best = %+v, want %+v", got, readings[1])
	}
	if i != len(readings) {
		t.Errorf("read called %d times, want %d", i, len(readings))
	}
}

func TestBestMinimumOne(t *testing.T) {
	calls := 0
	read := func() (Reading, error) {
		calls++
		return Reading{TSF: 7}, nil
	}
	for _, n := range []int{-3, 0, 1} {
		calls = 0
		got, err := Best(n, read)
		if err != nil || got.TSF != 7 || calls != 1 {
			t.Errorf("Best(%d) = %+v, %v with %d calls, want 1 call", n, got, err, calls)
		}
	}
}

func TestBestError(t *testing.T) {
	calls := 0
	read := func() (Reading, error) {
		calls++
		if calls == 2 {
			return Reading{}, errRead
		}
		return Reading{TSF: 1}, nil
	}
	got, err := Best(5, read)
	if err != errRead {
		t.Errorf("err = %v, want %v", err, errRead)
	}
	if got != (Reading{}) {
		t.Errorf("Best = %+v, want the zero Reading", got)
	}
	if calls != 2 {
		t.Errorf("read called %d times, want 2", calls)
	}
}

func TestBestAllocs(t *testing.T) {
	read := func() (Reading, error) { return Reading{TSF: 1, After: 10}, nil }
	if n := testing.AllocsPerRun(100, func() { Best(8, read) }); n != 0 {
		t.Errorf("Best allocates %v times, want 0", n)
	}
}
