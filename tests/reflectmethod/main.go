// Checks reflect's method lookup: Type.Method/MethodByName and
// Value.Method/MethodByName. Prints "reflectmethod ok" if every check passed.
package main

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

type node struct {
	ticks float64
	name  string
}

func (n *node) Process(delta float64) { n.ticks += delta }
func (n *node) Name() string          { return n.name }
func (n *node) Rename(s string)       { n.name = s }
func (n *node) Split(sep string) (string, string) {
	a, b, _ := strings.Cut(n.name, sep)
	return a, b
}
func (n *node) Join(sep string, parts ...string) string { return strings.Join(parts, sep) }
func (n *node) hidden()                                 {}

type celsius int

func (c celsius) F() int            { return int(c)*9/5 + 32 }
func (c celsius) Add(d int) celsius { return c + celsius(d) }

type big struct {
	A, B, C, D, E, F, G, H int64
}

func (b big) Sum(extra int64) int64 { return b.A + b.B + b.C + b.D + b.E + b.F + b.G + b.H + extra }

type outer struct {
	*node
	Tag string
}

var failed bool

func check(name string, got, want any) {
	if fmt.Sprint(got) != fmt.Sprint(want) {
		failed = true
		println("FAIL", name, "got", fmt.Sprint(got), "want", fmt.Sprint(want))
		return
	}
	println("ok", name)
}

func names(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumMethod(); i++ {
		out = append(out, t.Method(i).Name)
	}
	return out
}

func vals(vs []reflect.Value) []any {
	var out []any
	for _, v := range vs {
		out = append(out, v.Interface())
	}
	return out
}

func main() {
	pt := reflect.TypeOf(&node{})
	check("method names in order", names(pt), []string{"Join", "Name", "Process", "Rename", "Split"})
	m, ok := pt.MethodByName("Process")
	check("MethodByName found", fmt.Sprintf("%v %v %v", ok, m.Name, m.Index), "true Process 2")
	check("Method.Type has the receiver", m.Type, "func(*main.node, float64)")
	_, ok = pt.MethodByName("hidden")
	check("unexported not found", ok, false)
	_, ok = pt.MethodByName("Nope")
	check("missing not found", ok, false)

	n := &node{name: "left:right"}
	m.Func.Call([]reflect.Value{reflect.ValueOf(n), reflect.ValueOf(1.5)})
	check("Method.Func call", n.ticks, 1.5)

	v := reflect.ValueOf(n)
	v.MethodByName("Process").Call([]reflect.Value{reflect.ValueOf(2.0)})
	check("Value.MethodByName pointer receiver", n.ticks, 3.5)
	check("Value.Method results", vals(v.MethodByName("Split").Call([]reflect.Value{reflect.ValueOf(":")})), []any{"left", "right"})
	v.Method(3).Call([]reflect.Value{reflect.ValueOf("renamed")})
	check("Value.Method by index", n.Name(), "renamed")
	check("variadic method", vals(v.MethodByName("Join").Call([]reflect.Value{reflect.ValueOf("-"), reflect.ValueOf("a"), reflect.ValueOf("b")})), []any{"a-b"})
	check("missing method value", v.MethodByName("Nope").IsValid(), false)

	name := v.MethodByName("Name").Interface().(func() string)
	check("method value as a func", name(), "renamed")

	c := reflect.ValueOf(celsius(100))
	check("small value receiver", vals(c.MethodByName("F").Call(nil)), []any{212})
	check("value receiver result type", vals(c.MethodByName("Add").Call([]reflect.Value{reflect.ValueOf(5)})), []any{celsius(105)})
	check("value type method names", names(reflect.TypeOf(celsius(0))), []string{"Add", "F"})

	b := reflect.ValueOf(big{1, 2, 3, 4, 5, 6, 7, 8})
	check("large value receiver", vals(b.MethodByName("Sum").Call([]reflect.Value{reflect.ValueOf(int64(10))})), []any{int64(46)})
	bm, _ := reflect.TypeOf(big{}).MethodByName("Sum")
	check("large receiver Method.Func", vals(bm.Func.Call([]reflect.Value{b, reflect.ValueOf(int64(1))})), []any{int64(37)})

	o := &outer{node: &node{name: "inner"}, Tag: "t"}
	check("promoted method", vals(reflect.ValueOf(o).MethodByName("Name").Call(nil)), []any{"inner"})
	check("promoted names", names(reflect.TypeOf(o)), []string{"Join", "Name", "Process", "Rename", "Split"})

	// graphics.gd's shortcut: the method's func reinterpreted with a raw receiver.
	held := reflect.New(m.Type)
	held.Elem().Set(m.Func)
	tick := *(*func(unsafe.Pointer, float64))(held.UnsafePointer())
	n2 := &node{}
	tick(unsafe.Pointer(n2), 0.25)
	check("method func reinterpreted", n2.ticks, 0.25)

	if !failed {
		println("reflectmethod ok")
	}
}
