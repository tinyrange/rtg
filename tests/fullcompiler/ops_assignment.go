package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// :=
	x := 10
	if x != 10 { fmt.Printf("FAIL: :=\n"); passed = false }

	// =
	x = 20
	if x != 20 { fmt.Printf("FAIL: =\n"); passed = false }

	// +=
	x += 5
	if x != 25 { fmt.Printf("FAIL: +=\n"); passed = false }

	// -=
	x -= 10
	if x != 15 { fmt.Printf("FAIL: -=\n"); passed = false }

	// *=
	x *= 2
	if x != 30 { fmt.Printf("FAIL: *=\n"); passed = false }

	// /=
	x /= 3
	if x != 10 { fmt.Printf("FAIL: /=\n"); passed = false }

	// %=
	x %= 3
	if x != 1 { fmt.Printf("FAIL: %%=\n"); passed = false }

	// |=
	x = 0xF0
	x |= 0x0F
	if x != 0xFF { fmt.Printf("FAIL: |=\n"); passed = false }

	// &=
	x &= 0x0F
	if x != 0x0F { fmt.Printf("FAIL: &=\n"); passed = false }

	// ^=
	x ^= 0xFF
	if x != 0xF0 { fmt.Printf("FAIL: ^=\n"); passed = false }

	// <<=
	x = 1
	x <<= 8
	if x != 256 { fmt.Printf("FAIL: <<=\n"); passed = false }

	// >>=
	x >>= 4
	if x != 16 { fmt.Printf("FAIL: >>=\n"); passed = false }

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
