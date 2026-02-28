package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Nil pointer
	var p *int
	if p != nil {
		fmt.Printf("FAIL: nil pointer\n")
		passed = false
	}

	// Non-nil pointer
	x := 42
	p = &x
	if p == nil {
		fmt.Printf("FAIL: non-nil pointer\n")
		passed = false
	}

	// Nil slice
	var s []int
	if s != nil {
		fmt.Printf("FAIL: nil slice\n")
		passed = false
	}
	if len(s) != 0 {
		fmt.Printf("FAIL: nil slice len\n")
		passed = false
	}

	// Nil map
	var m map[string]int
	if m != nil {
		fmt.Printf("FAIL: nil map\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
