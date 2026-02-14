package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Basic int
	var a int = 42
	if a != 42 {
		fmt.Printf("FAIL: int literal\n")
		passed = false
	}

	// int32
	var b int32 = 100000
	if b != 100000 {
		fmt.Printf("FAIL: int32 literal\n")
		passed = false
	}

	// uint
	var c uint = 55
	if c != 55 {
		fmt.Printf("FAIL: uint literal\n")
		passed = false
	}

	// uint16
	var d uint16 = 65535
	if d != 65535 {
		fmt.Printf("FAIL: uint16 literal\n")
		passed = false
	}

	// uint32
	var e uint32 = 4294967295
	if e != 4294967295 {
		fmt.Printf("FAIL: uint32 literal\n")
		passed = false
	}

	// byte
	var f byte = 255
	if f != 255 {
		fmt.Printf("FAIL: byte literal\n")
		passed = false
	}

	// Arithmetic
	x := 10
	y := 3
	if x+y != 13 {
		fmt.Printf("FAIL: addition\n")
		passed = false
	}
	if x-y != 7 {
		fmt.Printf("FAIL: subtraction\n")
		passed = false
	}
	if x*y != 30 {
		fmt.Printf("FAIL: multiplication\n")
		passed = false
	}
	if x/y != 3 {
		fmt.Printf("FAIL: division\n")
		passed = false
	}
	if x%y != 1 {
		fmt.Printf("FAIL: modulo\n")
		passed = false
	}

	// Hex and octal literals
	hex := 0xFF
	if hex != 255 {
		fmt.Printf("FAIL: hex literal\n")
		passed = false
	}

	oct := 0777
	if oct != 511 {
		fmt.Printf("FAIL: octal literal\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
