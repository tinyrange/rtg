package main

import "fmt"

func main() {
	a := [2][2][2]int{{{1, 2}, {3, 4}}, {{5, 6}, {7, 8}}}
	b := a
	b[1][0][1] = 99
	if a[1][0][1] == 6 && b[1][0][1] == 99 {
		fmt.Print("PASS")
		return
	}
	panic("three-level fixed array assignment did not deep copy")
}
