package main

import (
	"fmt"
	"os"
)

func split(sum int) (int, int) {
	x := sum * 4 / 9
	y := sum - x
	return x, y
}

func main() {
	passed := true

	x, y := split(17)
	if x+y != 17 {
		fmt.Printf("FAIL: named return x=%d y=%d\n", x, y)
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
