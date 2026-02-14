package main

import (
	"fmt"
	"os"
)

func add(a int, b int) int {
	return a + b
}

func noReturn() {
	// does nothing
}

func greet(name string) string {
	return "hello " + name
}

func main() {
	passed := true

	if add(3, 4) != 7 {
		fmt.Printf("FAIL: add\n")
		passed = false
	}

	noReturn()

	if greet("world") != "hello world" {
		fmt.Printf("FAIL: greet\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
