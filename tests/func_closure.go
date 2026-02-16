package main

import (
	"fmt"
	"os"
)

func double(x int) int {
	return x * 2
}

func triple(x int) int {
	return x * 3
}

func main() {
	passed := true

	// Direct calls to named functions
	if double(5) != 10 {
		fmt.Printf("FAIL: double\n")
		passed = false
	}

	if triple(4) != 12 {
		fmt.Printf("FAIL: triple\n")
		passed = false
	}

	// Multiple calls
	if double(double(3)) != 12 {
		fmt.Printf("FAIL: nested call\n")
		passed = false
	}

	// Function result as argument
	if double(triple(2)) != 12 {
		fmt.Printf("FAIL: composed call\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
