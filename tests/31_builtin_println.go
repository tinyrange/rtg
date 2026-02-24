package main

import "fmt"

func main() {
	if 1 == 0 {
		println("unreachable")
	}
	fmt.Print("PASS")
}
