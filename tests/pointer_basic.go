package main

import (
	"fmt"
	"os"
)

func increment(p *int) {
	*p = *p + 1
}

func main() {
	passed := true

	// Basic pointer
	x := 42
	p := &x
	if *p != 42 {
		fmt.Printf("FAIL: deref\n")
		passed = false
	}

	// Modify through pointer
	*p = 100
	if x != 100 {
		fmt.Printf("FAIL: modify via ptr\n")
		passed = false
	}

	// Pointer to pointer
	pp := &p
	**pp = 200
	if x != 200 {
		fmt.Printf("FAIL: ptr to ptr\n")
		passed = false
	}

	// Pass pointer to function
	increment(&x)
	if x != 201 {
		fmt.Printf("FAIL: pass ptr to func x=%d\n", x)
		passed = false
	}

	// Nil pointer
	var np *int
	if np != nil {
		fmt.Printf("FAIL: nil ptr\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
