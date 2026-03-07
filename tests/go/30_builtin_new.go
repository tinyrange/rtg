package main

import "fmt"

type T struct{ X int }

func main() {
	p := new(T)
	if p != nil && p.X == 0 {
		fmt.Print("PASS")
		return
	}
	panic("new builtin failed")
}
