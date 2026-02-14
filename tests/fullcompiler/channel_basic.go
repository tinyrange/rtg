package main

import "fmt"

func main() {
	// Channels are expected to be unsupported
	ch := make(chan int)
	_ = ch
	fmt.Printf("PASS\n")
}
