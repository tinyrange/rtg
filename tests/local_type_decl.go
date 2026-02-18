package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	type Local struct {
		X int
	}

	v := Local{X: 11}
	if v.X != 11 {
		fmt.Printf("FAIL: local type composite literal\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
