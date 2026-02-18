package main

import "fmt"

func main() {
	fmt.Println("before")
	_ = string(65)
	fmt.Println("after")
}
