package builder

import (
	"bytes"
	"reflect"
	"testing"
)

func uleb(v uint64) []byte {
	var b []byte
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b = append(b, c|0x80)
			continue
		}
		return append(b, c)
	}
}

func wasmName(s string) []byte { return append(uleb(uint64(len(s))), s...) }

func TestWasmLinkSymbolsListsDefinedGlobalFunctionsAndData(t *testing.T) {
	var syms []byte
	syms = append(syms, uleb(5)...) // count
	// defined function "f"
	syms = append(syms, 0, 0)
	syms = append(syms, uleb(0)...)
	syms = append(syms, wasmName("f")...)
	// undefined function, no name
	syms = append(syms, 0, wasmSymbolUndefined)
	syms = append(syms, uleb(1)...)
	// local function "g"
	syms = append(syms, 0, wasmSymbolBindingLocal)
	syms = append(syms, uleb(2)...)
	syms = append(syms, wasmName("g")...)
	// defined data "d": segment, an offset past 32 bits, size
	syms = append(syms, 1, 0)
	syms = append(syms, wasmName("d")...)
	syms = append(syms, uleb(0)...)
	syms = append(syms, uleb(1<<40)...)
	syms = append(syms, uleb(8)...)
	// section symbol
	syms = append(syms, 3, 0)
	syms = append(syms, uleb(4)...)

	var linking []byte
	linking = append(linking, wasmName("linking")...)
	linking = append(linking, uleb(2)...) // version
	linking = append(linking, wasmSubsectionSymbolTable)
	linking = append(linking, uleb(uint64(len(syms)))...)
	linking = append(linking, syms...)

	// A memory64 import section first: limits flag 0x04 and 64-bit sizes.
	memory := []byte{1}
	memory = append(memory, wasmName("env")...)
	memory = append(memory, wasmName("__linear_memory")...)
	memory = append(memory, 2, 0x04)
	memory = append(memory, uleb(1<<33)...)

	obj := []byte("\x00asm\x01\x00\x00\x00")
	obj = append(obj, 2)
	obj = append(obj, uleb(uint64(len(memory)))...)
	obj = append(obj, memory...)
	obj = append(obj, 0)
	obj = append(obj, uleb(uint64(len(linking)))...)
	obj = append(obj, linking...)

	got, err := wasmLinkSymbols(bytes.NewReader(obj))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"f", "d"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWasmLinkSymbolsRejectsOtherFiles(t *testing.T) {
	if _, err := wasmLinkSymbols(bytes.NewReader([]byte("\x7fELF...."))); err != errNotWasm {
		t.Errorf("got %v, want errNotWasm", err)
	}
}
