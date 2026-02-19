package main

import "fmt"

func main() {
	a := [...]int{1, 2, 3}
	if len(a) == 3 && a[2] == 3 {
		fmt.Print("PASS")
		return
	}
	panic("array ellipsis length failed")
}
