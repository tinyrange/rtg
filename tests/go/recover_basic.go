package main

import "fmt"

func main() {
	// recover should be available as a predeclared builtin value.
	_ = recover
	fmt.Printf("PASS\n")
}
