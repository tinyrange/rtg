package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Multi-value assignment
	a, b := 1, 2
	if a != 1 || b != 2 {
		fmt.Printf("FAIL: multi assign\n")
		passed = false
	}

	// Swap
	a, b = b, a
	if a != 2 || b != 1 {
		fmt.Printf("FAIL: swap a=%d b=%d\n", a, b)
		passed = false
	}

	// Multi-assign with expressions
	x, y, z := 10, 20, 30
	if x != 10 || y != 20 || z != 30 {
		fmt.Printf("FAIL: triple assign\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
