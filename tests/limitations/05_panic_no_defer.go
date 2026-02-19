package main

import "fmt"

func main() {
	defer fmt.Println("deferred")
	panic("boom")
}
