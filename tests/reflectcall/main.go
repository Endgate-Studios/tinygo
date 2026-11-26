// Checks reflect.Value.Call and the func type's In and Out. Prints one line
// per check; "reflectcall ok" at the end only if every check passed.
package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type small struct{ A, B int32 }

type large struct {
	A, B, C, D, E, F, G, H int64
	S                      string
}

type counter struct{ n int }

func (c *counter) Add(d int) int { c.n += d; return c.n }

type shape interface{ Area() float64 }

type square float64

func (s square) Area() float64 { return float64(s * s) }

func add(a, b int) int                        { return a + b }
func concat(a string, b string) string        { return a + b }
func divmod(a, b int) (int, int, error)       { return a / b, a % b, nil }
func nothing()                                {}
func swapSmall(s small) small                 { return small{s.B, s.A} }
func sumLarge(l large) (int64, string)        { return l.A + l.B + l.C + l.D + l.E + l.F + l.G + l.H, l.S }
func makeLarge(n int64) large                 { return large{n, n, n, n, n, n, n, n, "made"} }
func area(s shape) float64                    { return s.Area() }
func join(sep string, parts ...string) string { return strings.Join(parts, sep) }
func fail(msg string) error                   { return errors.New(msg) }
func boom()                                   { panic("boom") }
func recv(c chan int) int                     { return <-c }

var failed bool

func check(name string, got, want any) {
	if fmt.Sprint(got) != fmt.Sprint(want) {
		failed = true
		println("FAIL", name, "got", fmt.Sprint(got), "want", fmt.Sprint(want))
		return
	}
	println("ok", name)
}

func call(f any, args ...any) []any {
	in := make([]reflect.Value, len(args))
	for i, a := range args {
		in[i] = reflect.ValueOf(a)
	}
	var out []any
	for _, v := range reflect.ValueOf(f).Call(in) {
		out = append(out, v.Interface())
	}
	return out
}

func main() {
	t := reflect.TypeOf(divmod)
	check("NumIn NumOut", fmt.Sprint(t.NumIn(), t.NumOut()), "2 3")
	check("In Out", fmt.Sprint(t.In(0), t.Out(1), t.Out(2)), "int int error")
	check("IsVariadic", fmt.Sprint(reflect.TypeOf(join).IsVariadic(), t.IsVariadic()), "true false")
	check("variadic In", reflect.TypeOf(join).In(1), "[]string")

	check("ints", call(add, 2, 3), []any{5})
	check("strings", call(concat, "go", "pher"), []any{"gopher"})
	check("three results", call(divmod, 17, 5), []any{3, 2, nil})
	check("no results", len(call(nothing)), 0)
	check("small struct", call(swapSmall, small{1, 2}), []any{small{2, 1}})
	check("large struct in", call(sumLarge, large{1, 2, 3, 4, 5, 6, 7, 8, "s"}), []any{int64(36), "s"})
	check("large struct out", call(makeLarge, int64(4)), []any{large{4, 4, 4, 4, 4, 4, 4, 4, "made"}})
	check("interface param", call(area, square(3)), []any{9.0})
	check("error result", call(fail, "bad")[0].(error).Error(), "bad")

	k := 10
	closure := func(x int) int { return x + k }
	check("closure", call(closure, 5), []any{15})

	c := &counter{}
	call(c.Add, 3)
	check("method value", call(c.Add, 4), []any{7})

	check("variadic Call", call(join, "-", "a", "b", "c"), []any{"a-b-c"})
	check("variadic none", call(join, ","), []any{""})
	slice := reflect.ValueOf(join).CallSlice([]reflect.Value{reflect.ValueOf("+"), reflect.ValueOf([]string{"x", "y"})})
	check("CallSlice", slice[0].Interface(), "x+y")

	func() {
		defer func() { check("panic through Call", recover(), "boom") }()
		call(boom)
	}()

	func() {
		defer func() { check("wrong arg count", recover() != nil, true) }()
		call(add, 1)
	}()

	ch := make(chan int)
	go func() { ch <- 42 }()
	check("callee that blocks", call(recv, ch), []any{42})

	if !failed {
		println("reflectcall ok")
	}
}
