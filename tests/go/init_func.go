package main

import (
	"fmt"
	"os"
)

var initialized = false

func init() {
	initialized = true
}

func main() {
	passed := true

	if !initialized {
		fmt.Printf("FAIL: init not called\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
