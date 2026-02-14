package main

import (
	"fmt"
	"os"
)

func main() {
	// float64 is expected to have limited or no support
	// This test documents the behavior
	passed := true

	// Try basic float literal
	var f float64 = 3.14
	if f == 0.0 {
		fmt.Printf("FAIL: float literal is zero\n")
		passed = false
	}

	// Float arithmetic
	a := 1.5
	b := 2.5
	c := a + b
	if c != 4.0 {
		fmt.Printf("FAIL: float add\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
