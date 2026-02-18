package main

import "fmt"

func main() {
	switch x := 2; x {
	case 2:
		fmt.Println("two")
	}
}
