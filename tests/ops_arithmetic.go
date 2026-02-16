package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Int arithmetic
	if 10+3 != 13 { fmt.Printf("FAIL: 10+3\n"); passed = false }
	if 10-3 != 7 { fmt.Printf("FAIL: 10-3\n"); passed = false }
	if 10*3 != 30 { fmt.Printf("FAIL: 10*3\n"); passed = false }
	if 10/3 != 3 { fmt.Printf("FAIL: 10/3\n"); passed = false }
	if 10%3 != 1 { fmt.Printf("FAIL: 10%%3\n"); passed = false }

	// Negative numbers
	if -5+3 != -2 { fmt.Printf("FAIL: -5+3\n"); passed = false }
	if -5*-3 != 15 { fmt.Printf("FAIL: -5*-3\n"); passed = false }

	// Int32 arithmetic
	var a int32 = 1000000
	var b int32 = 2000000
	if a+b != 3000000 { fmt.Printf("FAIL: int32 add\n"); passed = false }

	// Uint arithmetic
	var c uint = 100
	var d uint = 50
	if c-d != 50 { fmt.Printf("FAIL: uint sub\n"); passed = false }

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
