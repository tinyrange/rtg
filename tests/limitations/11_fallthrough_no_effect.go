package main

import "fmt"

func main() {
	sum := 0
	switch 1 {
	case 1:
		sum += 1
		fallthrough
	case 2:
		sum += 2
	}
	if sum == 3 {
		fmt.Print("PASS")
		return
	}
	panic("fallthrough failed")
}
