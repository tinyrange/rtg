package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Map literal
	m := map[string]int{
		"x": 10,
		"y": 20,
		"z": 30,
	}
	if m["x"] != 10 || m["y"] != 20 || m["z"] != 30 {
		fmt.Printf("FAIL: map literal\n")
		passed = false
	}

	// Empty map literal
	empty := map[string]int{}
	if len(empty) != 0 {
		fmt.Printf("FAIL: empty map literal\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
