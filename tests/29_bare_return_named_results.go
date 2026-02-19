package main

import "fmt"

func f() (a int) {
	a = 3
	return
}

func main() {
	if f() == 3 {
		fmt.Print("PASS")
		return
	}
	panic("bare return named result failed")
}
