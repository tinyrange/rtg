package main

import "fmt"

func main() {
	m := map[int]int{1: 1, 2: 2}
	clear(m)
	if len(m) == 0 {
		fmt.Print("PASS")
		return
	}
	panic("clear builtin failed")
}
