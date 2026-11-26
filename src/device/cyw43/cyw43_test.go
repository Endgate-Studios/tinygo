package cyw43

import "testing"

func TestProcessNVRAM(t *testing.T) {
	out := processNVRAM([]byte("# comment\nboardtype=0x0727\r\n\n  boardrev=0x1202  \n"))
	want := "boardtype=0x0727\x00boardrev=0x1202\x00\x00"
	body := out[:len(out)-4]
	if string(body[:len(want)]) != want || len(body)%4 != 0 {
		t.Fatalf("body %q", body)
	}
	words := uint32(len(body) / 4)
	if tok := le32(out[len(out)-4:]); tok != (^words)<<16|words {
		t.Errorf("token %#x for %d words", tok, words)
	}
}

func TestEvents(t *testing.T) {
	frame := make([]byte, eventHeaderSkip+48)
	frame[12], frame[13] = 0x88, 0x6c
	m := frame[eventHeaderSkip:]
	var e eventState
	m[3], m[7] = 1, eventLink
	e.handle(frame)
	if !e.link {
		t.Fatal("link event didn't set link")
	}
	m[3], m[7], m[11] = 0, eventPSKSup, supKeyed
	e.handle(frame)
	if !e.keyed || e.authFail {
		t.Fatal("PSK keyed event")
	}
	m[7], m[11] = eventSetSSID, 1
	e.handle(frame)
	if !e.setSSID || e.ssidOK {
		t.Fatal("failed SET_SSID event")
	}
}

func TestFrameQueue(t *testing.T) {
	var q frameQueue
	for i := 0; i < 20; i++ {
		q.put([]byte{byte(i)})
	}
	if q.lost != 4 {
		t.Fatalf("lost %d", q.lost)
	}
	var b [4]byte
	for i := 0; i < 16; i++ {
		n, ok := q.get(b[:])
		if !ok || n != 1 || b[0] != byte(i) {
			t.Fatalf("frame %d: %v %d %d", i, ok, n, b[0])
		}
	}
	if _, ok := q.get(b[:]); ok {
		t.Fatal("queue not empty")
	}
}
