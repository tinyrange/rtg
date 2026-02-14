package main

import "fmt"

func main() {
	// recover() is expected to be unsupported in RTG
	// Just test if it compiles
	_ = recover
	fmt.Printf("PASS\n")
}
