package main

import (
	"fmt"
	"os"
)

const Pi = 3
const Message = "hello"
const (
	A = 10
	B = 20
	C = A + B
)

func main() {
	passed := true

	if Pi != 3 {
		fmt.Printf("FAIL: Pi\n")
		passed = false
	}

	if Message != "hello" {
		fmt.Printf("FAIL: Message\n")
		passed = false
	}

	if A != 10 || B != 20 || C != 30 {
		fmt.Printf("FAIL: const group\n")
		passed = false
	}

	// Typed constant
	const typed int = 100
	if typed != 100 {
		fmt.Printf("FAIL: typed const\n")
		passed = false
	}

	// Local constant
	const local = "world"
	if local != "world" {
		fmt.Printf("FAIL: local const\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
