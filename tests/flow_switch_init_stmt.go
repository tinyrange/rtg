package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true
	seen := 0

	switch x := 2; x {
	case 1:
		seen = 1
	case 2:
		seen = 2
	default:
		seen = 3
	}

	if seen != 2 {
		fmt.Printf("FAIL: switch init seen=%d\n", seen)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
