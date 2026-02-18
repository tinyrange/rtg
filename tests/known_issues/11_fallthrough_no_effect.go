package main

import "fmt"

func main() {
	switch 1 {
	case 1:
		fmt.Println("one")
		fallthrough
	case 2:
		fmt.Println("two")
	}
}
