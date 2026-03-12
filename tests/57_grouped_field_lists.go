package main

import (
	"fmt"
	"os"
)

type PairOp func(a, b int) (int, int)

type Pairer interface {
	Pair(a, b int) int
}

type Calc struct{}

func (c Calc) Pair(a, b int) int {
	return a + b
}

func split(a, b int) (sum, diff int) {
	sum = a + b
	diff = a - b
	return
}

func main() {
	passed := true

	var op PairOp = split
	sum, diff := op(9, 4)
	if sum != 13 || diff != 5 {
		fmt.Printf("FAIL: func type grouped fields\n")
		passed = false
	}

	var pairer Pairer = Calc{}
	_ = pairer

	sum, diff = split(6, 1)
	if sum != 7 || diff != 5 {
		fmt.Printf("FAIL: function grouped params/results\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
