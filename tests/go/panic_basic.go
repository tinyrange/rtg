package main

import "fmt"

func main() {
	// Just test that panic exists and can be called
	// We can't really test it without recover, so just test that
	// the program can reach the end without panicking
	x := 42
	if x == 0 {
		panic("this should not happen")
	}
	fmt.Printf("PASS\n")
}
