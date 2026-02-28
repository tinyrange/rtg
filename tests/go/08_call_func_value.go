package main

import "fmt"

type Fn func(int) int

func inc(x int) int { return x + 1 }

func main() {
	var f Fn = inc
	if f(1) == 2 {
		fmt.Print("PASS")
		return
	}
	panic("function value call failed")
}
