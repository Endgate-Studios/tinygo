package heap

import (
	"testing"
	"unsafe"

	"runtime/memory"
)

var (
	_ memory.Allocator = (*Heap)(nil)
	_ memory.Freer     = (*Heap)(nil)
	_ memory.Collector = (*Heap)(nil)
)

type testFreer struct{ freed []unsafe.Pointer }

func (f *testFreer) Free(p unsafe.Pointer) { f.freed = append(f.freed, p) }

type testCollector struct{ cycles int }

func (c *testCollector) Collect() { c.cycles++ }

func TestNewRejectsNilDependencies(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
	}{
		{name: "nil freer", opt: WithFreer(nil)},
		{name: "nil collector", opt: WithCollector(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opt); err == nil {
				t.Error("New() error = nil, want error")
			}
		})
	}
}

func TestHeapDelegatesToInjectedDependencies(t *testing.T) {
	f := &testFreer{}
	c := &testCollector{}
	h, err := New(WithFreer(f), WithCollector(c))
	if err != nil {
		t.Fatal(err)
	}

	type node struct {
		next *node
		val  int
	}
	n := memory.New[node](h)
	n.next = &node{val: 7} // a GC pointer in GC heap memory is permitted
	h.Collect()
	if n.next.val != 7 {
		t.Fatal("pointer held in heap memory lost")
	}
	memory.Free(h, n)
	h.Free(nil)

	if len(f.freed) != 1 || f.freed[0] != unsafe.Pointer(n) {
		t.Errorf("freed = %v, want [%p]", f.freed, n)
	}
	if c.cycles != 1 {
		t.Errorf("cycles = %d, want 1", c.cycles)
	}
}

func TestAllocAlignment(t *testing.T) {
	h, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, align := range []uintptr{0, 1, 8, 64, 256} {
		p := h.Alloc(10, align)
		if p == nil || (align > 0 && uintptr(p)%align != 0) {
			t.Errorf("Alloc(10, %d) = %p", align, p)
		}
	}
	if h.Alloc(10, 12) != nil {
		t.Error("Alloc with non-power-of-two alignment != nil")
	}
}
