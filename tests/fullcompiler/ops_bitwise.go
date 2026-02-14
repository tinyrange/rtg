package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// AND
	if 0xFF&0x0F != 0x0F { fmt.Printf("FAIL: AND\n"); passed = false }

	// OR
	if 0xF0|0x0F != 0xFF { fmt.Printf("FAIL: OR\n"); passed = false }

	// XOR
	if 0xFF^0x0F != 0xF0 { fmt.Printf("FAIL: XOR\n"); passed = false }

	// Left shift
	if 1<<8 != 256 { fmt.Printf("FAIL: left shift\n"); passed = false }

	// Right shift
	if 256>>8 != 1 { fmt.Printf("FAIL: right shift\n"); passed = false }

	// Bit clear (AND NOT)
	if 0xFF&^0x0F != 0xF0 { fmt.Printf("FAIL: bit clear\n"); passed = false }

	// Shift by variable
	n := 4
	if 1<<n != 16 { fmt.Printf("FAIL: shift by var\n"); passed = false }

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
