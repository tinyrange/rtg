package main

import "fmt"

type Fn func(int) int

func inc(x int) int { return x + 1 }

func main() {
	var f Fn = inc
	fmt.Println(f(1))
}
