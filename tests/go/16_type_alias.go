package main

import "fmt"

type MyInt = int

func main() {
	var x MyInt = 7
	if x == 7 {
		fmt.Print("PASS")
		return
	}
	panic("type alias failed")
}
