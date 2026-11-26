package central

import (
	"bytes"
	"testing"

	"runtime/memory/arena"
	"tinygo/net/bluetooth/hci"
)

type controller struct{ cmds []hci.Command }

func (c *controller) WriteHCI(b []byte) error {
	cmd, err := hci.ParseCommand(append([]byte(nil), b...))
	if err != nil {
		return err
	}
	c.cmds = append(c.cmds, cmd)
	return nil
}

type reports struct {
	names []string
	rssi  []int8
}

func (r *reports) Advertisement(a hci.AdvReport) {
	name, _ := ADField(a.Data, 0x09)
	r.names = append(r.names, string(name))
	r.rssi = append(r.rssi, a.RSSI)
}

func event(t *testing.T, c *Central, code uint8, params []byte) error {
	t.Helper()
	var b [260]byte
	n, err := hci.PutEvent(b[:], code, params)
	if err != nil {
		t.Fatal(err)
	}
	return c.HandleHCI(b[:n], 0)
}

func complete(t *testing.T, c *Central, op hci.Opcode, status byte) error {
	t.Helper()
	return event(t, c, hci.EvtCommandComplete, []byte{1, byte(op), byte(op >> 8), status})
}

// advReport is an LE Advertising Report with one report: ADV_IND from a random address.
func advReport(name string, rssi int8) []byte {
	data := append([]byte{2, 0x01, 0x06, byte(1 + len(name)), 0x09}, name...)
	p := []byte{hci.SubAdvertisingReport, 1, hci.AdvInd, hci.AddrRandom, 1, 2, 3, 4, 5, 0xc6, byte(len(data))}
	p = append(p, data...)
	return append(p, byte(rssi))
}

func newCentral(t *testing.T) (*Central, *controller) {
	t.Helper()
	tr := &controller{}
	c, err := New(newMem(), tr, testConfig(ScanConfig{Active: true, Interval: 0x60, Window: 0x30, OwnAddrType: hci.AddrRandom, FilterDuplicates: true}))
	if err != nil {
		t.Fatal(err)
	}
	return c, tr
}

func TestScan(t *testing.T) {
	c, tr := newCentral(t)
	r := &reports{}
	if err := c.Scan(r); err != nil {
		t.Fatal(err)
	}
	if len(tr.cmds) != 1 || tr.cmds[0].Op != hci.OpLESetScanParams || tr.cmds[0].Params[0] != 1 {
		t.Fatalf("first command %+v, want active scan parameters", tr.cmds)
	}
	if err := c.Scan(r); err != ErrBusy {
		t.Errorf("second Scan while starting: %v", err)
	}
	complete(t, c, hci.OpLESetScanParams, 0)
	if len(tr.cmds) != 2 || tr.cmds[1].Op != hci.OpLESetScanEnable || !bytes.Equal(tr.cmds[1].Params, []byte{1, 1}) {
		t.Fatalf("second command %+v, want enable with duplicate filter", tr.cmds)
	}
	complete(t, c, hci.OpLESetScanEnable, 0)
	if c.State() != StateScanning {
		t.Fatalf("state %d, want scanning", c.State())
	}
	for _, rep := range []struct {
		name string
		rssi int8
	}{{"tiny", -40}, {"core", -71}} {
		if err := event(t, c, hci.EvtLEMeta, advReport(rep.name, rep.rssi)); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.names) != 2 || r.names[0] != "tiny" || r.names[1] != "core" || r.rssi[1] != -71 {
		t.Errorf("reports %v %v", r.names, r.rssi)
	}
	if err := c.StopScan(); err != nil || tr.cmds[2].Op != hci.OpLESetScanEnable || tr.cmds[2].Params[0] != 0 {
		t.Fatalf("stop: %v, %+v", err, tr.cmds)
	}
	complete(t, c, hci.OpLESetScanEnable, 0)
	event(t, c, hci.EvtLEMeta, advReport("late", -50))
	if c.State() != StateIdle || len(r.names) != 2 {
		t.Errorf("state %d, reports %v after stop", c.State(), r.names)
	}
}

func TestScanRejected(t *testing.T) {
	c, _ := newCentral(t)
	c.Scan(&reports{})
	// 0x12 is Invalid HCI Command Parameters (Core spec v5.4 Vol 1 Part F 2.18).
	if err := complete(t, c, hci.OpLESetScanParams, 0x12); err != ErrController || c.State() != StateFailed || c.Status() != 0x12 {
		t.Errorf("err %v, state %d, status %#x", err, c.State(), c.Status())
	}
}

func newMem() *arena.Heap {
	mem, _ := arena.NewHeap(make([]byte, 4096))
	return mem
}

func testConfig(sc ScanConfig) Config {
	return Config{Scan: sc, MaxMTU: 64, MaxACL: 27, ACLBuffers: 4}
}

func TestConfigAndCredits(t *testing.T) {
	tr := &controller{}
	for _, sc := range []ScanConfig{{Interval: 0x10, Window: 3}, {Interval: 0x10, Window: 0x20}} {
		c, err := New(newMem(), tr, testConfig(sc))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Scan(&reports{}); err != ErrConfig {
			t.Errorf("scan config %+v: %v", sc, err)
		}
	}
	if _, err := New(nil, nil, Config{}); err != ErrNilDependency {
		t.Errorf("nil memory and transport: %v", err)
	}
	if _, err := New(newMem(), tr, Config{MaxACL: 26}); err != ErrConfig {
		t.Errorf("MaxACL 26: %v", err)
	}
	c, _ := newCentral(t)
	c.flow.TookCommand() // no credit left
	if err := c.Scan(&reports{}); err != ErrNoCredits || c.State() != StateIdle {
		t.Errorf("scan with no credit: %v, state %d", err, c.State())
	}
}

func TestADField(t *testing.T) {
	data := []byte{2, 0x01, 0x06, 5, 0x09, 't', 'i', 'n', 'y', 0, 0, 0}
	tests := []struct {
		name string
		data []byte
		typ  uint8
		want string
		ok   bool
	}{
		{name: "name", data: data, typ: 0x09, want: "tiny", ok: true},
		{name: "flags", data: data, typ: 0x01, want: "\x06", ok: true},
		{name: "missing, stops at padding", data: data, typ: 0xff},
		{name: "field longer than data", data: []byte{9, 0x09, 'x'}, typ: 0x09},
		{name: "empty", data: nil, typ: 0x09},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ADField(tt.data, tt.typ)
			if ok != tt.ok || string(got) != tt.want {
				t.Errorf("ADField = %q, %v, want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
