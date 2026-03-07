package main

import (
	"fmt"
	"os"
)

func main() {
	passed := true

	x := 1
	{
		x := 2
		if x != 2 {
			fmt.Printf("FAIL: inner shadow\n")
			passed = false
		}
	}
	if x != 1 {
		fmt.Printf("FAIL: outer after shadow x=%d\n", x)
		passed = false
	}

	// Shadow in if
	y := 10
	if true {
		y := 20
		if y != 20 {
			fmt.Printf("FAIL: if shadow\n")
			passed = false
		}
	}
	if y != 10 {
		fmt.Printf("FAIL: outer after if shadow\n")
		passed = false
	}

	// Shadow in for
	z := 100
	for i := 0; i < 1; i++ {
		z := 200
		if z != 200 {
			fmt.Printf("FAIL: for shadow\n")
			passed = false
		}
	}
	if z != 100 {
		fmt.Printf("FAIL: outer after for shadow\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
