package main

import "fmt"

func main() {
	// defer is currently unsupported and should produce a clear compile-time error.
	defer fmt.Print("PASS")
}
