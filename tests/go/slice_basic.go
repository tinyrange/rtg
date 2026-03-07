package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Make
	s := make([]int, 3)
	if len(s) != 3 {
		fmt.Printf("FAIL: make len\n")
		passed = false
	}

	// Assignment and indexing
	s[0] = 10
	s[1] = 20
	s[2] = 30
	if s[0] != 10 || s[1] != 20 || s[2] != 30 {
		fmt.Printf("FAIL: index assign\n")
		passed = false
	}

	// Make with cap
	s2 := make([]int, 2, 10)
	if len(s2) != 2 {
		fmt.Printf("FAIL: make with cap len\n")
		passed = false
	}
	if cap(s2) != 10 {
		fmt.Printf("FAIL: make with cap cap\n")
		passed = false
	}

	// Slice literal
	s3 := []int{100, 200, 300}
	if len(s3) != 3 || s3[0] != 100 {
		fmt.Printf("FAIL: slice literal\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
