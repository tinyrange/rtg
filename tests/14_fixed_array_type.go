package main

import "fmt"

func main() {
	a := [3]int{1, 2, 3}
	if a[1] == 2 && len(a) == 3 {
		fmt.Print("PASS")
		return
	}
	panic("fixed array failed")
}
