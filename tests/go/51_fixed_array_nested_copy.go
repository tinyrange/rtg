package main

import "fmt"

func mutate(a [2][3]int) {
	a[1][0] = 88
}

func snapshot(src [2][3]int) [2][3]int {
	return src
}

func main() {
	a := [2][3]int{{1, 2, 3}, {4, 5, 6}}
	b := a
	b[0][1] = 99
	if a[0][1] != 2 {
		panic("nested fixed array assignment did not deep copy")
	}

	mutate(a)
	if a[1][0] != 4 {
		panic("nested fixed array argument was not passed by value")
	}

	v := snapshot(a)
	v[1][2] = 77
	if a[1][2] != 6 {
		panic("nested fixed array return value aliases source")
	}

	fmt.Print("PASS")
}
