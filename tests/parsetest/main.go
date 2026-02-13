package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	content, err := os.ReadFile("tools/Buildfile")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	lines := strings.Split(string(content), "\n")
	fmt.Printf("lines: %d\n", len(lines))

	// Test range loop
	for i, line := range lines {
		fmt.Printf("range line %d: %s\n", i, line)
	}

	// Test direct index
	fmt.Printf("direct lines[0]: %s\n", lines[0])

	// Test local copy
	x := lines[0]
	fmt.Printf("local x: %s\n", x)
}
