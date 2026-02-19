package main

import "fmt"

const x = 1_000

func main() {
	if x == 1000 {
		fmt.Print("PASS")
		return
	}
	panic("numeric separator failed")
}
