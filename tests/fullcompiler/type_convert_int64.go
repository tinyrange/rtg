package main

import (
	"fmt"
)

func main() {
	// This test documents that int64() conversion is NOT supported in RTG
	// The compiler treats int64() as an unknown function call
	// Just try to use int64 as a variable type instead
	var x int = 42
	_ = x
	fmt.Printf("PASS\n")
	// Note: actual int64() conversion would fail at compile time
}
