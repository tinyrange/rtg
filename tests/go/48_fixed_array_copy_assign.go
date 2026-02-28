package main

import "fmt"

func main() {
	a := [3]int{1, 2, 3}
	b := a
	b[1] = 9
	if a[1] == 2 && b[1] == 9 {
		fmt.Print("PASS")
		return
	}
	panic("fixed array assignment did not copy")
}
