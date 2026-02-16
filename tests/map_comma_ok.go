package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	m := make(map[string]int)
	m["key"] = 42

	// Key exists
	v, ok := m["key"]
	if !ok || v != 42 {
		fmt.Printf("FAIL: comma ok exists\n")
		passed = false
	}

	// Key doesn't exist
	v2, ok2 := m["nokey"]
	if ok2 {
		fmt.Printf("FAIL: comma ok missing\n")
		passed = false
	}
	if v2 != 0 {
		fmt.Printf("FAIL: comma ok zero\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
