package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Unary minus
	x := 5
	if -x != -5 { fmt.Printf("FAIL: unary minus\n"); passed = false }

	// Unary NOT
	if !true != false { fmt.Printf("FAIL: unary not\n"); passed = false }
	if !false != true { fmt.Printf("FAIL: unary not 2\n"); passed = false }

	// Unary XOR (bitwise complement)
	var b byte = 0
	if ^b != 255 { fmt.Printf("FAIL: unary xor byte\n"); passed = false }

	// Address-of and dereference
	y := 42
	p := &y
	if *p != 42 { fmt.Printf("FAIL: deref\n"); passed = false }
	*p = 100
	if y != 100 { fmt.Printf("FAIL: deref assign\n"); passed = false }

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
