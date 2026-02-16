package main

import (
	"fmt"
	"os"
)

func typeString(i interface{}) string {
	switch i.(type) {
	case int:
		return "int"
	case string:
		return "string"
	case bool:
		return "bool"
	case []int:
		return "[]int"
	default:
		return "unknown"
	}
}

func main() {
	passed := true

	if typeString(42) != "int" {
		fmt.Printf("FAIL: typeswitch int\n")
		passed = false
	}
	if typeString("hi") != "string" {
		fmt.Printf("FAIL: typeswitch string\n")
		passed = false
	}
	if typeString(true) != "bool" {
		fmt.Printf("FAIL: typeswitch bool\n")
		passed = false
	}
	if typeString([]int{1, 2}) != "[]int" {
		fmt.Printf("FAIL: typeswitch slice\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
