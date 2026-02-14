package main

import (
	"fmt"
	"os"
)

func divmod(a, b int) (int, int) {
	return a / b, a % b
}

func swap(a, b string) (string, string) {
	return b, a
}

func main() {
	passed := true

	q, r := divmod(17, 5)
	if q != 3 || r != 2 {
		fmt.Printf("FAIL: divmod q=%d r=%d\n", q, r)
		passed = false
	}

	x, y := swap("hello", "world")
	if x != "world" || y != "hello" {
		fmt.Printf("FAIL: swap\n")
		passed = false
	}

	// Ignore second return
	q2, _ := divmod(10, 3)
	if q2 != 3 {
		fmt.Printf("FAIL: ignore return\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
