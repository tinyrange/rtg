package main

import "fmt"

func main() {
	if string(65) == "A" {
		fmt.Print("PASS")
		return
	}
	panic("int to string conversion failed")
}
