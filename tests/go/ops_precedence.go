package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Multiplication before addition
	if 2+3*4 != 14 { fmt.Printf("FAIL: 2+3*4\n"); passed = false }
	if (2+3)*4 != 20 { fmt.Printf("FAIL: (2+3)*4\n"); passed = false }

	// Division before subtraction
	if 10-6/2 != 7 { fmt.Printf("FAIL: 10-6/2\n"); passed = false }

	// Shift vs addition: << has higher precedence than + in Go, so 1<<4+1 = (1<<4)+1 = 17
	if 1<<4+1 != 17 { fmt.Printf("FAIL: 1<<4+1\n"); passed = false }

	// Comparison vs logical
	if !(1 < 2 && 3 < 4) { fmt.Printf("FAIL: comparison && comparison\n"); passed = false }

	// Nested parens
	if ((2+3)*(4+5)) != 45 { fmt.Printf("FAIL: nested parens\n"); passed = false }

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
