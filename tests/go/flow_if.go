package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Basic if
	x := 10
	if x == 10 {
		// ok
	} else {
		fmt.Printf("FAIL: basic if\n")
		passed = false
	}

	// If-else
	if x == 5 {
		fmt.Printf("FAIL: if-else took wrong branch\n")
		passed = false
	} else {
		// ok
	}

	// If-else if-else
	if x == 5 {
		fmt.Printf("FAIL: else-if chain\n")
		passed = false
	} else if x == 10 {
		// ok
	} else {
		fmt.Printf("FAIL: else-if chain default\n")
		passed = false
	}

	// If with init statement
	if y := x * 2; y == 20 {
		// ok
	} else {
		fmt.Printf("FAIL: if with init\n")
		passed = false
	}

	// Nested if
	if x > 5 {
		if x < 15 {
			// ok
		} else {
			fmt.Printf("FAIL: nested if\n")
			passed = false
		}
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
