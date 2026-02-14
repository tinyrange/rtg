package main

import (
	"fmt"
	"os"
)

func describe(i interface{}) string {
	switch i.(type) {
	case int:
		return "int"
	case string:
		return "string"
	case bool:
		return "bool"
	default:
		return "unknown"
	}
}

func main() {
	passed := true

	// Box int
	var i interface{} = 42
	if describe(i) != "int" {
		fmt.Printf("FAIL: box int\n")
		passed = false
	}

	// Box string
	i = "hello"
	if describe(i) != "string" {
		fmt.Printf("FAIL: box string\n")
		passed = false
	}

	// Box bool
	i = true
	if describe(i) != "bool" {
		fmt.Printf("FAIL: box bool\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
