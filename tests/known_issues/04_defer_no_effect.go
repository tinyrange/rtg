package main

import "fmt"

func main() {
	defer fmt.Println("deferred")
	fmt.Println("body")
}
