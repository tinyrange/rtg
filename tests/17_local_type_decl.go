package main

import "fmt"

func main() {
	type T struct{ X int }
	v := T{X: 1}
	if v.X == 1 {
		fmt.Print("PASS")
		return
	}
	panic("local type decl failed")
}
