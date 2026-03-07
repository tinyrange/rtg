package main

import (
	"fmt"
	"os"
)

func appendDigit(dst *string, n int) {
	*dst = *dst + string([]byte{byte('0' + n)})
}

func deferSimple() (order string) {
	appendDigit(&order, 0)
	defer appendDigit(&order, 3)
	defer appendDigit(&order, 2)
	defer appendDigit(&order, 1)
	return
}

func deferLoop() (order string) {
	i := 0
	for i < 3 {
		defer appendDigit(&order, i)
		i++
	}
	return
}

func bump(v *int) {
	*v = *v + 1
}

func deferNamedExplicit() (v int) {
	defer bump(&v)
	return 41
}

func deferVariadicIfaceCapture() (out int) {
	captured := 7
	fn := func(fixed int, values ...interface{}) {
		out = captured*100 + fixed*10 + len(values)
	}
	defer fn(3, 11, 22)
	return
}

type deferRecorder struct {
	out   *int
	scale int
}

func (r deferRecorder) apply(fixed int, values ...interface{}) {
	*r.out = r.scale*100 + fixed*10 + len(values)
}

func deferVariadicIfaceMethodValue() (out int) {
	r := deferRecorder{out: &out, scale: 5}
	method := r.apply
	defer method(4, 11, 22, 33)
	return
}

func main() {
	passed := true
	simple := deferSimple()
	if simple != "0123" {
		fmt.Printf("FAIL: defer simple got=%s\n", simple)
		passed = false
	}
	loop := deferLoop()
	if loop != "210" {
		fmt.Printf("FAIL: defer loop got=%s\n", loop)
		passed = false
	}
	named := deferNamedExplicit()
	if named != 42 {
		fmt.Printf("FAIL: defer named explicit got=%d\n", named)
		passed = false
	}
	capture := deferVariadicIfaceCapture()
	if capture != 732 {
		fmt.Printf("FAIL: defer variadic iface capture got=%d\n", capture)
		passed = false
	}
	method := deferVariadicIfaceMethodValue()
	if method != 543 {
		fmt.Printf("FAIL: defer variadic iface method got=%d\n", method)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
