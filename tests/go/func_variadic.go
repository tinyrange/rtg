package main

import (
	"fmt"
	"os"
)

func sum(nums ...int) int {
	total := 0
	for _, n := range nums {
		total += n
	}
	return total
}

func main() {
	passed := true

	if sum(1, 2, 3) != 6 {
		fmt.Printf("FAIL: variadic sum\n")
		passed = false
	}

	if sum() != 0 {
		fmt.Printf("FAIL: variadic empty\n")
		passed = false
	}

	// Spread operator
	nums := []int{4, 5, 6}
	if sum(nums...) != 15 {
		fmt.Printf("FAIL: variadic spread\n")
		passed = false
	}

	if passed {
		fmt.Printf("PASS\n")
	} else {
		os.Exit(1)
	}
}
