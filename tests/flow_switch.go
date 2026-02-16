package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	// Switch with tag
	x := 2
	result := ""
	switch x {
	case 1:
		result = "one"
	case 2:
		result = "two"
	case 3:
		result = "three"
	default:
		result = "other"
	}
	if result != "two" {
		fmt.Printf("FAIL: switch tag result=%s\n", result)
		passed = false
	}

	// Switch default
	switch 99 {
	case 1:
		result = "one"
	default:
		result = "default"
	}
	if result != "default" {
		fmt.Printf("FAIL: switch default\n")
		passed = false
	}

	// Switch without tag (bool switch)
	y := 15
	switch {
	case y < 10:
		result = "small"
	case y < 20:
		result = "medium"
	default:
		result = "large"
	}
	if result != "medium" {
		fmt.Printf("FAIL: switch no tag result=%s\n", result)
		passed = false
	}

	// Switch on string
	s := "hello"
	switch s {
	case "world":
		result = "world"
	case "hello":
		result = "hello"
	default:
		result = "unknown"
	}
	if result != "hello" {
		fmt.Printf("FAIL: switch string\n")
		passed = false
	}

	// Multiple case values
	z := 3
	matched := false
	switch z {
	case 1, 2, 3:
		matched = true
	case 4, 5:
		matched = false
	}
	if !matched {
		fmt.Printf("FAIL: switch multi-case\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
