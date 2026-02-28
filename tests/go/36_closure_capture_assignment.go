package main

import "fmt"

func main() {
	x := 0
	f := func() { x = 42 }
	f()
	if x == 42 {
		fmt.Print("PASS")
		return
	}
	panic("closure assignment failed")
}
