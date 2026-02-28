package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Make and set
	m := make(map[string]int)
	m["one"] = 1
	m["two"] = 2
	m["three"] = 3

	// Get
	if m["one"] != 1 {
		fmt.Printf("FAIL: get\n")
		passed = false
	}
	if m["two"] != 2 {
		fmt.Printf("FAIL: get two\n")
		passed = false
	}

	// Len
	if len(m) != 3 {
		fmt.Printf("FAIL: len=%d\n", len(m))
		passed = false
	}

	// Delete
	delete(m, "two")
	if len(m) != 2 {
		fmt.Printf("FAIL: delete len=%d\n", len(m))
		passed = false
	}

	// Missing key returns zero value
	if m["missing"] != 0 {
		fmt.Printf("FAIL: missing key\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
