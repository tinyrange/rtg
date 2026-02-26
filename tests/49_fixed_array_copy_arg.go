package main

import "fmt"

func mutate(a [3]int) {
	a[0] = 99
}

func main() {
	a := [3]int{1, 2, 3}
	mutate(a)
	if a[0] == 1 {
		fmt.Print("PASS")
		return
	}
	panic("fixed array argument was not passed by value")
}
