package main

import (
	"fmt"
	"os"
)

var globalInt = 42
var globalStr = "hello"
var globalSlice = []int{1, 2, 3}

func modifyGlobal() {
	globalInt = 100
}

func main() {
	passed := true

	if globalInt != 42 {
		fmt.Printf("FAIL: global int\n")
		passed = false
	}
	if globalStr != "hello" {
		fmt.Printf("FAIL: global str\n")
		passed = false
	}
	if len(globalSlice) != 3 {
		fmt.Printf("FAIL: global slice\n")
		passed = false
	}

	modifyGlobal()
	if globalInt != 100 {
		fmt.Printf("FAIL: modified global\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
