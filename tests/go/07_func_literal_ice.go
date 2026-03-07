package main

import "fmt"

func main() {
	f := func() int { return 2 }
	if f() == 2 {
		fmt.Print("PASS")
		return
	}
	panic("func literal call failed")
}
