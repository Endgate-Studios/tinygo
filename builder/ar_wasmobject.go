package builder

// The exported symbols of a wasm object file, from its "linking" custom
// section (WebAssembly tool conventions, Linking.md). Other sections are
// skipped by their length, so wasm64 (memory64) objects read the same.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

const (
	wasmSymbolFunction = 0
	wasmSymbolData     = 1
	wasmSymbolGlobal   = 2
	wasmSymbolSection  = 3
	wasmSymbolTag      = 4
	wasmSymbolTable    = 5

	wasmSymbolBindingLocal = 0x02
	wasmSymbolUndefined    = 0x10
	wasmSymbolExplicitName = 0x40

	wasmSubsectionSymbolTable = 8
)

var errNotWasm = errors.New("not a wasm object")

// wasmLinkSymbols returns the names of the defined, non-local function and
// data symbols of a wasm object: the ones an archive's index lists.
func wasmLinkSymbols(r io.Reader) ([]string, error) {
	br := bufio.NewReader(r)
	var header [8]byte
	if _, err := io.ReadFull(br, header[:]); err != nil || !bytes.Equal(header[:4], []byte("\x00asm")) {
		return nil, errNotWasm
	}
	for {
		id, err := br.ReadByte()
		if err == io.EOF {
			return nil, nil // no linking section: nothing to index
		}
		if err != nil {
			return nil, err
		}
		size, err := readULEB(br)
		if err != nil {
			return nil, err
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(br, payload); err != nil {
			return nil, fmt.Errorf("section %d: %w", id, err)
		}
		if id != 0 {
			continue
		}
		p := bytes.NewReader(payload)
		name, err := readWasmName(p)
		if err != nil {
			return nil, err
		}
		if name == "linking" {
			return readLinkingSymbols(p)
		}
	}
}

func readLinkingSymbols(r *bytes.Reader) ([]string, error) {
	if _, err := readULEB(r); err != nil { // version
		return nil, err
	}
	for r.Len() > 0 {
		kind, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		size, err := readULEB(r)
		if err != nil {
			return nil, err
		}
		if kind != wasmSubsectionSymbolTable {
			if _, err := r.Seek(int64(size), io.SeekCurrent); err != nil {
				return nil, err
			}
			continue
		}
		count, err := readULEB(r)
		if err != nil {
			return nil, err
		}
		var names []string
		for range count {
			name, export, err := readWasmSymbol(r)
			if err != nil {
				return nil, err
			}
			if export {
				names = append(names, name)
			}
		}
		return names, nil
	}
	return nil, nil
}

// readWasmSymbol reads one symbol info: its name, and whether an archive's
// index lists it (a defined, non-local function or data symbol).
func readWasmSymbol(r *bytes.Reader) (name string, export bool, err error) {
	kind, err := readULEB(r)
	if err != nil {
		return "", false, err
	}
	flags, err := readULEB(r)
	if err != nil {
		return "", false, err
	}
	defined := flags&wasmSymbolUndefined == 0
	switch kind {
	case wasmSymbolFunction, wasmSymbolGlobal, wasmSymbolTag, wasmSymbolTable:
		if _, err := readULEB(r); err != nil { // index
			return "", false, err
		}
		if defined || flags&wasmSymbolExplicitName != 0 {
			if name, err = readWasmName(r); err != nil {
				return "", false, err
			}
		}
	case wasmSymbolData:
		if name, err = readWasmName(r); err != nil {
			return "", false, err
		}
		if defined {
			for range 3 { // segment index, offset, size
				if _, err := readULEB(r); err != nil {
					return "", false, err
				}
			}
		}
	case wasmSymbolSection:
		if _, err := readULEB(r); err != nil {
			return "", false, err
		}
	default:
		return "", false, fmt.Errorf("unknown wasm symbol kind %d", kind)
	}
	export = defined && flags&wasmSymbolBindingLocal == 0 && (kind == wasmSymbolFunction || kind == wasmSymbolData)
	return name, export, nil
}

func readWasmName(r io.ByteReader) (string, error) {
	n, err := readULEB(r)
	if err != nil {
		return "", err
	}
	b := make([]byte, n)
	for i := range b {
		if b[i], err = r.ReadByte(); err != nil {
			return "", err
		}
	}
	return string(b), nil
}

func readULEB(r io.ByteReader) (uint64, error) {
	var v uint64
	for shift := uint(0); shift < 64; shift += 7 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		v |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, errors.New("LEB128 value too long")
}
