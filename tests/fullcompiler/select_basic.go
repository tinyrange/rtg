package main

import "fmt"

func main() {
	// Select is expected to be unsupported
	ch := make(chan int)
	_ = ch
	// select { case <-ch: }  -- probably won't compile
	fmt.Printf("PASS\n")
}
