package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Basic string
	s := "hello"
	if len(s) != 5 {
		fmt.Printf("FAIL: string len\n")
		passed = false
	}

	// Concatenation
	s2 := s + " world"
	if len(s2) != 11 {
		fmt.Printf("FAIL: string concat len\n")
		passed = false
	}
	if s2 != "hello world" {
		fmt.Printf("FAIL: string concat value\n")
		passed = false
	}

	// Indexing
	if s[0] != 'h' {
		fmt.Printf("FAIL: string index 0\n")
		passed = false
	}
	if s[4] != 'o' {
		fmt.Printf("FAIL: string index 4\n")
		passed = false
	}

	// Escape sequences
	tab := "a\tb"
	if len(tab) != 3 {
		fmt.Printf("FAIL: tab escape len\n")
		passed = false
	}
	nl := "a\nb"
	if len(nl) != 3 {
		fmt.Printf("FAIL: newline escape len\n")
		passed = false
	}

	// Empty string
	empty := ""
	if len(empty) != 0 {
		fmt.Printf("FAIL: empty string len\n")
		passed = false
	}

	// String comparison
	if "abc" >= "abd" {
		fmt.Printf("FAIL: string comparison\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
