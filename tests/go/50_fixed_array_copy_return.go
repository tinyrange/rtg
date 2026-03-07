package main

import "fmt"

var g = [2]int{1, 2}

func snapshot() [2]int {
	return g
}

func main() {
	v := snapshot()
	v[0] = 7
	if g[0] == 1 {
		fmt.Print("PASS")
		return
	}
	panic("fixed array return value aliases source")
}
