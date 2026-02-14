package main

import "fmt"

func main() {
	// Goroutines are expected to be unsupported in RTG
	// Just test if 'go' keyword compiles (it probably won't)
	// If this test fails with COMPILE_ERROR, that's expected
	done := make(chan bool)
	go func() {
		done <- true
	}()
	<-done
	fmt.Printf("PASS\n")
}
