package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBundle(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{"a.bin": "firmware", "b.txt": "nvram=1\n", "c.clm_blob": "x"}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "bundle.bin")
	if err := writeBundle(dir, out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "TGFW" || binary.LittleEndian.Uint32(b[4:]) != 3 {
		t.Fatalf("header %q", b[:8])
	}
	for i := 0; i < 3; i++ {
		e := b[8+64*i:]
		n := 0
		for e[n] != 0 {
			n++
		}
		off := binary.LittleEndian.Uint32(e[56:])
		size := binary.LittleEndian.Uint32(e[60:])
		if off%16 != 0 || string(b[off:off+size]) != files[string(e[:n])] {
			t.Errorf("%s at %d size %d", e[:n], off, size)
		}
	}
}
