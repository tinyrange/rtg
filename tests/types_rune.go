package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Rune literal
	r := 'A'
	if r != 65 {
		fmt.Printf("FAIL: rune A != 65\n")
		passed = false
	}

	// Escape runes
	nl := '\n'
	if nl != 10 {
		fmt.Printf("FAIL: rune newline != 10\n")
		passed = false
	}

	tab := '\t'
	if tab != 9 {
		fmt.Printf("FAIL: rune tab != 9\n")
		passed = false
	}

	// Rune arithmetic
	r2 := 'a' + 1
	if r2 != 'b' {
		fmt.Printf("FAIL: rune arithmetic\n")
		passed = false
	}

	// Rune from string
	s := "Hello"
	if s[0] != 'H' {
		fmt.Printf("FAIL: string byte as rune\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
